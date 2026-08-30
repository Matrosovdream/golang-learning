package main

// pageTemplate wraps the rendered body in a self-contained, print-tuned page.
// Everything is inlined: headless Chrome loads the file straight off disk with
// no network, so there are no webfonts and no external stylesheet.
const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
  @page { size: A4; margin: 11mm 9mm; }

  * { box-sizing: border-box; }

  html { -webkit-print-color-adjust: exact; print-color-adjust: exact; }

  body {
    margin: 0;
    font: 9.5pt/1.45 -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    color: #1b1f24;
  }

  h1 {
    font-size: 16pt;
    margin: 0 0 2px;
    padding-bottom: 5px;
    border-bottom: 2px solid #1b1f24;
    letter-spacing: -0.2px;
  }

  h2 {
    font-size: 11pt;
    letter-spacing: 0.1px;
    margin: 15px 0 6px;
    padding-bottom: 3px;
    border-bottom: 1px solid #c9d1d9;
    break-after: avoid;
  }

  h3 {
    font-size: 9.5pt;
    margin: 11px 0 4px;
    color: #384049;
    break-after: avoid;
  }

  p { margin: 5px 0; }

  ul { margin: 5px 0; padding-left: 17px; }
  li { margin: 2px 0; }

  pre {
    margin: 5px 0;
    padding: 6px 8px;
    background: #f6f8fa;
    border: 1px solid #d8dee4;
    border-radius: 4px;
    break-inside: avoid;
    overflow: hidden;
  }

  pre code {
    font: 7.9pt/1.4 ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
    background: none;
    padding: 0;
    white-space: pre;
    color: #1b1f24;
  }

  code {
    font: 8.4pt ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
    background: #eceff2;
    padding: 0.5px 3px;
    border-radius: 3px;
  }

  table {
    width: 100%%;
    border-collapse: collapse;
    margin: 6px 0;
    font-size: 8.6pt;
    break-inside: avoid;
  }

  th, td {
    border: 1px solid #d8dee4;
    padding: 3px 6px;
    text-align: left;
    vertical-align: top;
  }

  th { background: #eceff2; font-weight: 600; }

  a { color: #0550ae; text-decoration: none; }

  hr { border: 0; border-top: 1px solid #d8dee4; margin: 12px 0; }

  strong { font-weight: 650; }
</style>
</head>
<body>
%s</body>
</html>
`
