---
toc: false
---

<div class="hero">

# Flaky Tests (v2)

Test Failures: ${(failureCount)[0].count}

Using n=${numberOfJobs[0].count} runs of `go test`.

### Flakiness Score: ${Math.round(calculateOddsAnyTestFails(failurePercent.map(x => x.failure_percent))*100)}%
Lower is better. This is roughly the odds that any given run will fail because of a flaky test

</div>

<div class="grid grid-cols-2" style="grid-auto-rows: 704px;">
  <div class="card">${resize(boxChartOfFailures)}</div>
  <div class="card">${resize(chartFlakesOverTime)}</div>
</div>

---

```js
import * as Plot from "npm:@observablehq/plot";
function boxChartOfFailures(width, height) {
 return Plot.plot({
    title: "Flakiness by test and OS",
    marks: [
      Plot.barY(failedTests, Plot.groupX({
        y: "count",
        },{
        x: d => `${d.Package}.${d.Test} ${d.OS}`,
        // y: "Failures",
        fill: "OS",
        title: (d) => `${d.PackageTest} (${d.OS})`,
        sort: { x: "y", reverse: true } // Sort by failures
      }))
    ],
    x: {
      // label: "Test Name",
      label: "",
      labelAngle: 45, // Rotate labels by 45 degrees
      tickRotate: 45 // Ensure tick labels are rotated
    },
    y: {
      label: "Number of Failures"
    },
    color: {
      legend: true, // Display legend for OS
      label: "Operating System"
    },
    height: height-250,
    width,
  });
}
```

```js
const db = FileAttachment("data/test_results.db").sqlite()

const formatFailedTest = ({ Test, Package, Outputs, OS, Go, Failures, WorkflowID }) => {
  return html`
<details>
<summary>${Package}.${Test} ${OS ?? ""} ${Go ?? ""}</summary>
<a href="http://github.com/libp2p/go-libp2p/actions/runs/${WorkflowID}">Workflow Link</a>
<pre>
${Outputs}
</pre>
</details>

`;
}
```

```js
let failedTests = await db.sql`
  SELECT
      t1.Package,
      t1.Test,
      (t1.Package || "." || t1.Test) as PackageTest,
      t1.OS,
      t1.WorkflowID,
      GROUP_CONCAT(t2.Output, '') AS Outputs
  FROM
      test_results t1
  JOIN
      test_results t2
  ON
      t1.Package = t2.Package
      AND t1.Test = t2.Test
      AND t1.BatchInsertTime = t2.BatchInsertTime
      AND t2.Action = 'output'
  WHERE
      t1.Action = 'fail'
  GROUP BY
      t1.Package,
      t1.Test,
      t1.OS,
      t1.Go,
      t1.WorkflowID
      ;
`;
display(failedTests)
failedTests = keepOnlyLeafTests(failedTests)
display(failedTests)

const x_ = `SELECT
    tr_output.Package,
    tr_output.Test,
    tr_output.OS,
    Failures,
    tr_output.WorkflowID,
    tr_output.BatchInsertTime,
    GROUP_CONCAT(tr_output.Output,  "") AS Outputs
FROM
    test_results tr_fail
JOIN
    test_results tr_output
ON
    tr_fail.Test = tr_output.Test
    AND tr_fail.BatchInsertTime = tr_output.BatchInsertTime
    AND tr_fail.Package = tr_output.Package
JOIN (
    SELECT
        Package,
        Test,
        COUNT(*) AS Failures
    FROM
        test_results
    WHERE
        Action = 'fail'
    GROUP BY
        Package,
        Test
) fail_count
ON
    tr_output.Package = fail_count.Package
    AND tr_output.Test = fail_count.Test
WHERE
    tr_fail.Action = 'fail'
    AND tr_output.Test != ''
GROUP BY
    tr_output.BatchInsertTime,
    tr_output.Package,
    tr_output.Test,
    tr_output.OS
ORDER BY
    fail_count.failures DESC,
    MIN(tr_output.Time);`;

```

## Logs of failed tests
Filter by testname:
```js
const testFilter = view(Inputs.text({
    label: "Filter",
    placeholder: "Filter failed tests",
    // datalist: failedTests.map(({Package, Test}) => `${Package}.${Test}`)
}))
```

```js
const failedFormattedTests = failedTests
  .filter(({Package, Test}) => `${Package}.${Test}`.includes(testFilter))
  .map(formatFailedTest)
```

<div> ${failedFormattedTests} </div>

```js
const failureCount = await db.sql`SELECT count(*) as count from test_results where Action = 'fail';`
```

```js
const numberOfJobs = await db.sql`SELECT COUNT(DISTINCT BatchInsertTime) as count from test_results`
```


```js
let failurePercent = await db.sql`SELECT
    Test,
    Package,
    (Package || "." || Test) as PackageTest,
    OS,
    Go,
    Time,
    SUM(CASE WHEN Action = 'pass' THEN 1 ELSE 0 END) AS pass_count,
    SUM(CASE WHEN Action = 'fail' THEN 1 ELSE 0 END) AS fail_count,
    CAST(SUM(CASE WHEN Action = 'fail' THEN 1 ELSE 0 END) AS FLOAT) /
    SUM(CASE WHEN Action = 'pass' OR Action = 'fail' THEN 1 ELSE 0 END) AS failure_percent
FROM
    test_results
GROUP BY
    Test,
    Package,
    OS,
    Go
HAVING
    pass_count > 0 AND fail_count > 0 AND Test != '';`

failurePercent = keepOnlyLeafTests(failurePercent)
display(failurePercent)

function keepOnlyLeafTests(data) {
  console.log(data)
  data.sort((a, b) => {
    const aStr = a.OS+a.Go+a.PackageTest
    const bStr = b.OS+b.Go+b.PackageTest
    return aStr.localeCompare(bStr)
  })
  let out = []
  for (let i = 0; i < data.length; i++) {
    let current = data[i]
    let next = data[i+1]
    if (next && next.PackageTest.startsWith(current.PackageTest) && next.OS === current.OS && next.Go === current.Go && next.WorkflowID === current.WorkflowID) {
      continue
    }
    out.push(current)
  }
  return out;
}

failurePercent = keepOnlyLeafTests(failurePercent)

// display(Inputs.table(failurePercent))
```

```js
const flakesOverTime = await db.sql`SELECT
    Test,
    Package,
    strftime('%Y-%m-%d', BatchInsertTime) AS BatchDay,
    SUM(CASE WHEN Action = 'pass' THEN 1 ELSE 0 END) AS pass_count,
    SUM(CASE WHEN Action = 'fail' THEN 1 ELSE 0 END) AS fail_count,
    CAST(SUM(CASE WHEN Action = 'fail' THEN 1 ELSE 0 END) AS FLOAT) /
    SUM(CASE WHEN Action = 'pass' OR Action = 'fail' THEN 1 ELSE 0 END) AS failure_percent
FROM
    test_results
GROUP BY
    Test,
    Package,
    BatchDay
HAVING
    pass_count > 0 AND fail_count > 0 AND Test != '';`

function chartFlakesOverTime(width, height) {
  flakesOverTime.forEach(d => {
    d.Series = `${d.Package}.${d.Test}`;
  });

  const chart = Plot.plot({
    title: "Flakiness over time",
    marks: [
      Plot.line(flakesOverTime, {
        x: "BatchDay",
        y: "failure_percent",
        z: "Series",
        stroke: "Series", // Different lines for different Package+Test combinations
        title: (d) =>
          `${d.Series}: ${d.failure_percent.toFixed(2)}% failure on ${
            d.BatchDay
          }`
      }),
      Plot.dot(flakesOverTime, {
        x: "BatchDay",
        y: "failure_percent",
        stroke: "Series",
        fill: "Series",
        r: 4, // Radius of the dots
        title: (d) =>
          `${d.Series}: ${d.failure_percent.toFixed(2)}% failure on ${
            d.BatchDay
          }`
      })
    ],
    x: {
      label: "Date",
      type: "time",
      labelAngle: 45, // Rotate labels by 45 degrees
      tickRotate: 45, // Ensure tick labels are rotated
      tickFormat: (x) => new Date(x) // Format the date on the x-axis
    },
    y: {
      label: "Failure Percent",
      tickFormat: (d) => `${(d * 100).toFixed(2)}%` // Format y-axis as percentage
    },
    color: {
      legend: true, // Show legend for different Series
      label: "Package + Test"
    },
    height: height-250,
    // insetBottom: 250,
    // marginBottom: 250,
    width
  });

  return chart;
}
```

```js
// Monte Carlo style simulation. Easier than doing the math :P
// Pass in an array of likelihood each test fails. e.g [0.1, 0.2] would mean job A has a failure rate of 0.1, and job B has a failure rate of 0.2
function calculateOddsAnyTestFails(testCases) {
  return 1-testCases.reduce((acc, v) => v*(1-acc))
}
```
