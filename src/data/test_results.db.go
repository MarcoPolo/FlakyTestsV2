package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

const GoTestWorkflowID = "go-test.yml"

type actionRun struct {
	ID           int    `json:"id"`
	ArtifactsURL string `json:"artifacts_url"`
}

type actionRuns struct {
	TotalCount   int         `json:"total_count"`
	WorkflowRuns []actionRun `json:"workflow_runs"`
}

type artifact struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	ArchiveDownloadURL string `json:"archive_download_url"`
	WorkflowRun        struct {
		ID int `json:"id"`
	} `json:"workflow_run"`
	dbFileName string
}

type artifacts struct {
	TotalCount int        `json:"total_count"`
	Artifacts  []artifact `json:"artifacts"`
}

func main() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	g := &github{auth: githubToken}
	res, err := g.Get("/repos/libp2p/go-libp2p/actions/workflows/" + GoTestWorkflowID + "/runs?branch=master")
	if err != nil {
		log.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	var parsed actionRuns
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		log.Fatal(err)
	}

	var allArtifacts artifacts
	for _, run := range parsed.WorkflowRuns {
		res, err = g.Get(run.ArtifactsURL)
		if err != nil {
			log.Fatal(err)
		}
		j := json.NewDecoder(res.Body)
		var as artifacts
		err := j.Decode(&as)
		if err != nil {
			log.Fatal(err)
		}
		for i := range as.Artifacts {
			a := &as.Artifacts[i]
			err = g.lazyGetArtifact(a)
			if err != nil {
				log.Fatal(err)
			}
		}
		allArtifacts.Artifacts = append(allArtifacts.Artifacts, as.Artifacts...)
		allArtifacts.TotalCount += as.TotalCount
	}
	mergeTables(allArtifacts)

	f, err := os.Open("artifacts/merged.db")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	io.Copy(os.Stdout, f)
}

type github struct {
	auth string
}

func (g *github) lazyGetArtifact(a *artifact) error {
	if _, err := os.Stat("artifacts"); os.IsNotExist(err) {
		os.Mkdir("artifacts", 0755)
	}
	fileName := "artifacts/" + strconv.Itoa(a.WorkflowRun.ID) + "_" + a.Name
	a.dbFileName = fileName
	_, err := os.Stat(fileName)
	if err == nil {
		return nil
	}
	res, err := g.Get(a.ArchiveDownloadURL)
	if err != nil {
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	// Unzip response body
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return err
	}
	f, err := r.Open("test_results.db")
	if err != nil {
		return err
	}
	defer f.Close()
	db, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	persistedFile, err := os.Create("artifacts/" + strconv.Itoa(a.WorkflowRun.ID) + "_" + a.Name)
	if err != nil {
		return err
	}
	defer persistedFile.Close()

	_, err = persistedFile.Write(db)
	if err != nil {
		return err
	}

	err = addMetadata(a)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func (g *github) Get(url string) (*http.Response, error) {
	if !strings.Contains(url, "https://") {
		url = "https://api.github.com" + url
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+g.auth)
	req.Header.Add("Accept", " application/vnd.github+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	return http.DefaultClient.Do(req)
}

func addMetadata(a *artifact) error {
	metadataParts := strings.Split(path.Base(a.dbFileName), "_")
	if len(metadataParts) < 3 {
		return fmt.Errorf("invalid dbFileName: %s", a.dbFileName)
	}
	workflowID := metadataParts[0]
	os := metadataParts[1]
	goVersion := metadataParts[2]

	db, err := sql.Open("sqlite", a.dbFileName)
	if err != nil {
		return err
	}
	defer db.Close()

	var aggregateErr error
	_, err = db.Exec("ALTER TABLE test_results ADD COLUMN WorkflowID TEXT")
	aggregateErr = errors.Join(aggregateErr, err)
	_, err = db.Exec("ALTER TABLE test_results ADD COLUMN OS TEXT")
	aggregateErr = errors.Join(aggregateErr, err)
	_, err = db.Exec("ALTER TABLE test_results ADD COLUMN Go TEXT")
	aggregateErr = errors.Join(aggregateErr, err)
	_, err = db.Exec("UPDATE test_results SET WorkflowID = ?", workflowID)
	aggregateErr = errors.Join(aggregateErr, err)
	_, err = db.Exec("UPDATE test_results SET OS = ?", os)
	aggregateErr = errors.Join(aggregateErr, err)
	_, err = db.Exec("UPDATE test_results SET Go = ?", goVersion)
	aggregateErr = errors.Join(aggregateErr, err)

	if aggregateErr != nil {
		return aggregateErr
	}
	return nil
}

func mergeTables(allArtifacts artifacts) error {
	if len(allArtifacts.Artifacts) == 0 {
		return fmt.Errorf("no artifacts to merge")
	}

	os.Remove("artifacts/merged.db")
	f, err := os.Create("artifacts/merged.db")
	if err != nil {
		return err
	}

	dbBytes, err := os.ReadFile(allArtifacts.Artifacts[0].dbFileName)
	if err != nil {
		return err
	}

	allArtifacts.Artifacts = allArtifacts.Artifacts[1:]
	allArtifacts.TotalCount--
	_, err = f.Write(dbBytes)
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	mergedDB, err := sql.Open("sqlite", "artifacts/merged.db")
	if err != nil {
		return err
	}
	defer mergedDB.Close()

	// Assuming `allArtifacts` is already defined and contains the list of artifacts
	for _, a := range allArtifacts.Artifacts {
		log.Printf("Merging %s into merged.db\n", a.dbFileName)
		err := func() error {
			db, err := sql.Open("sqlite", a.dbFileName)
			if err != nil {
				return err
			}
			defer db.Close()

			// Read the columns from the test_results table
			rows, err := db.Query("PRAGMA table_info(test_results)")
			if err != nil {
				return err
			}

			var columns []string
			for rows.Next() {
				var cid int
				var name string
				var type_ string
				var notnull int
				var dflt_value *string
				var pk int
				err = rows.Scan(&cid, &name, &type_, &notnull, &dflt_value, &pk)
				if err != nil {
					return err
				}
				columns = append(columns, name)
			}
			rows.Close()

			// Prepare an insert statement template for mergedDB
			insertQuery := fmt.Sprintf("INSERT INTO test_results (%s) VALUES (%s)",
				strings.Join(columns, ", "),
				strings.Repeat("?, ", len(columns)-1)+"?")

			// Fetch rows from the current db's test_results table
			rows, err = db.Query("SELECT * FROM test_results")
			if err != nil {
				return err
			}
			defer rows.Close()

			tx, err := mergedDB.Begin()
			if err != nil {
				return err
			}
			tx.Prepare(insertQuery)

			// Insert each row into the mergedDB's test_results table
			for rows.Next() {
				var values []interface{}
				for range columns {
					var v interface{}
					values = append(values, &v)
				}
				err = rows.Scan(values...)
				if err != nil {
					return err
				}

				_, err = tx.Exec(insertQuery, values...)
				if err != nil {
					return err
				}
			}
			tx.Commit()
			return nil
		}()
		if err != nil {
			return err
		}
	}

	log.Println("Data merged successfully.")
	return nil
}
