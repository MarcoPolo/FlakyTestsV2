---
theme: dashboard
title: Slow Tests
toc: false
---

# Slow Tests

```js
const db = FileAttachment("data/test_results.db").sqlite()
```


```js
const testsWithDuration = await db.sql`
SELECT
    Package, Test, OS,
    AVG(Elapsed) AS AvgElapsed
FROM
    test_results
WHERE
    Elapsed > 0.1
    AND (Action = 'pass' OR Action = 'fail')
    AND Test != ''
GROUP BY
    Package, Test, OS
ORDER BY
    Elapsed DESC
LIMIT 10;
`
```

```js
const slowTestsChart = (width) => {
  return Plot.plot({
    marks: [
      Plot.barX(testsWithDuration, {
        x: "AvgElapsed",
        y: ({Package, Test, OS}) => `${Package}.${Test} ${OS}`,
        href: ({Package, Test, OS}) => `#test=${Package}.${Test}`,
        fill: "Package",
        sort: { y: "x", reverse: true },
      })
    ],
    title: "Slow tests",
    width,
    height: 400,
    marginLeft: 400,
    x: { label: "Average Elapsed Time (s)" },
    y: { label: "Package" }
  })
}
```

<div class="grid grid-cols-1">
  <div class="card">
    ${resize(slowTestsChart)}
  </div>
</div>

```js
const hash = Generators.observe(notify => {
  const hashchange = () => notify(location.hash);
  hashchange();
  addEventListener("hashchange", hashchange);
  return () => removeEventListener("hashchange", hashchange);
})
```

```js
function getParam(k) {
  const parts = hash.substring(1).split("=")
  for (let i = 0; i < parts.length; i++) {
    if (parts[i] == k) {
      return parts[i + 1]
    }
  }
}

const testFilter = view(Inputs.text({
    label: "Find logs for a test",
    value: getParam("test"),
    width: 600,
}))
```

```js
const testFilterWithPercent = testFilter === "" ? `""` : `%${testFilter}%`
let testLogs = await db.sql`
  SELECT
      t1.Package,
      t1.Test,
      t1.OS,
      t1.Go,
      GROUP_CONCAT(t1.Output, '') AS Outputs,
      t1.WorkflowID,
      t2.Elapsed
  FROM
      test_results t1
  LEFT JOIN
      test_results t2
  ON
      t1.Package = t2.Package
      AND t1.Test = t2.Test
      AND t1.BatchInsertTime = t2.BatchInsertTime
  WHERE
      t1.Action == 'output'
      AND t2.Elapsed > 1
      AND (t1.Package || '.' || t1.Test) LIKE ${testFilterWithPercent}
  GROUP BY
      t1.BatchInsertTime
      ;`;
testLogs.sort((a, b) => b.Elapsed - a.Elapsed)
display(testLogs)
```

```js
const formatTestLog = ({ Test, Package, Outputs, OS, Go, WorkflowID, Elapsed }) => {
  return html`
<details>
<summary>${Package}.${Test} ${OS ?? ""} ${Go ?? ""} (${Elapsed}s)</summary>
<a href="http://github.com/libp2p/go-libp2p/actions/runs/${WorkflowID}">Workflow Link</a>
<pre>
${Outputs}
</pre>
</details>
`;
}
```

<div>
${testLogs.map(formatTestLog)}
</div>
