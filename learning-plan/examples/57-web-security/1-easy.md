# Step 57 — Web Application Security · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

The two biggest classes of web bug — **injection** and **XSS** — plus not trusting the request's size or shape.

---

## 1. SQL injection vs parameterized queries

`🟢 easy` · *injection*

SQL injection happens when input is glued into a query as **code**. The parameterized form sends the query and the value on **separate channels**, so `x' OR '1'='1` can only ever be data — it never changes the query's structure.

**Steps:**

1. Concatenate the input into a `WHERE` clause — the quote closes early and the `OR` is now code.
2. The parameterized form keeps a `$1` placeholder; the value travels as a separate arg.
3. Rule: never build SQL by string concatenation.

```go
package main

import "fmt"

func main() {
	userInput := "x' OR '1'='1"

	// DANGEROUS: building SQL by string concatenation. The input becomes CODE, so
	// the WHERE clause is subverted and matches every row.
	bad := "SELECT * FROM users WHERE name = '" + userInput + "'"
	fmt.Println("built query:", bad)

	// SAFE: a parameterized query. The driver sends the SQL and the VALUE separately,
	// so the input can only ever be data, never code.
	//   db.Query("SELECT * FROM users WHERE name = $1", userInput)
	query := "SELECT * FROM users WHERE name = $1"
	args := []any{userInput}
	fmt.Printf("safe query:  %s   args: %q\n", query, args)
}
```

**Output:**

```
built query: SELECT * FROM users WHERE name = 'x' OR '1'='1'
safe query:  SELECT * FROM users WHERE name = $1   args: ["x' OR '1'='1"]
```

---

## 2. XSS: html/template vs text/template

`🟢 easy` · *xss*

`html/template` **auto-escapes** interpolated values for the HTML context, neutralizing injected markup. `text/template` (same API!) does **not** — using it to build HTML is a cross-site-scripting hole. The package you import is the whole difference.

**Steps:**

1. Render hostile input with `html/template` — the `<script>` becomes inert entities.
2. Render the same input with `text/template` — it comes through raw.
3. Always use `html/template` for anything served as HTML.

```go
package main

import (
	"html/template"
	"os"
	tt "text/template"
)

func main() {
	user := `<script>alert('xss')</script>`

	// html/template AUTO-ESCAPES for the HTML context — the script is neutralized.
	ht := template.Must(template.New("h").Parse(`<p>Hello {{.}}</p>`))
	ht.Execute(os.Stdout, user)
	os.Stdout.WriteString("\n")

	// text/template does NOT escape — using it to build HTML is an XSS hole.
	xt := tt.Must(tt.New("t").Parse(`<p>Hello {{.}}</p>`))
	xt.Execute(os.Stdout, user)
	os.Stdout.WriteString("\n")
}
```

**Output:**

```
<p>Hello &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;</p>
<p>Hello <script>alert('xss')</script></p>
```

---

## 3. Contextual output escaping

`🟢 easy` · *xss*

`html/template`'s superpower is **contextual** escaping: it knows whether a value lands in an HTML body, an attribute, a `<script>`, or a URL, and escapes each **differently**. You don't choose the encoding — the template engine does, correctly, per position.

**Steps:**

1. Put the same value in four contexts in one template.
2. Note HTML-entity escaping in the body/attr, JS-string escaping in `<script>`, percent-encoding in the URL.
3. This is why you never hand-escape — the engine can't be fooled by context.

```go
package main

import (
	"html/template"
	"os"
)

func main() {
	user := `a "b" <c> 'd'`
	// The SAME value is escaped DIFFERENTLY depending on where it lands: HTML body,
	// attribute, JS string, or URL query. html/template picks the right one per context.
	t := template.Must(template.New("x").Parse(
		`body: <p>{{.}}</p>` + "\n" +
			`attr: <img alt="{{.}}">` + "\n" +
			`js:   <script>var x = {{.}};</script>` + "\n" +
			`url:  <a href="/s?q={{.}}">link</a>`))
	t.Execute(os.Stdout, user)
	os.Stdout.WriteString("\n")
}
```

**Output:**

```
body: <p>a &#34;b&#34; &lt;c&gt; &#39;d&#39;</p>
attr: <img alt="a &#34;b&#34; &lt;c&gt; &#39;d&#39;">
js:   <script>var x = "a \"b\" \u003cc\u003e 'd'";</script>
url:  <a href="/s?q=a%20%22b%22%20%3cc%3e%20%27d%27">link</a>
```

---

## 4. The template.HTML bypass trap

`🟢 easy` · *xss*

`template.HTML` (and `template.JS`/`template.URL`) tells the engine "this is already safe — don't escape it." Wrapping **user input** in it re-opens XSS. Only ever mark content **you** produced as safe.

**Steps:**

1. A plain string is escaped (safe).
2. The same string wrapped in `template.HTML` is emitted raw — the `<script>` runs.
3. Treat these types as "I personally vouch this is safe", never for untrusted input.

```go
package main

import (
	"html/template"
	"os"
)

func main() {
	user := `<b>bold</b><script>alert(1)</script>`
	t := template.Must(template.New("x").Parse(`<div>{{.}}</div>`))

	// A normal string is escaped (safe).
	t.Execute(os.Stdout, user)
	os.Stdout.WriteString("\n")

	// template.HTML tells the engine "trust this, don't escape it". If the value is
	// user-controlled you've just reopened XSS. Only ever wrap content YOU produced.
	t.Execute(os.Stdout, template.HTML(user))
	os.Stdout.WriteString("\n")
}
```

**Output:**

```
<div>&lt;b&gt;bold&lt;/b&gt;&lt;script&gt;alert(1)&lt;/script&gt;</div>
<div><b>bold</b><script>alert(1)</script></div>
```

---

## 5. Command injection and os/exec

`🟢 easy` · *injection*

`exec.Command(prog, args...)` runs a program with its arguments passed **directly to the process** — there's no shell, so `;`, `|`, `$()` and friends are just literal characters in one argument. Command injection only happens if you build a string and hand it to a shell yourself.

**Steps:**

1. `exec.Command("echo", userInput)` — the whole `"hello; rm -rf /"` is a single arg.
2. Nothing is interpreted; there's no shell involved.
3. The dangerous anti-pattern (`sh -c "…"+input`) is shown as a string only — never run it.

```go
package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func main() {
	userInput := "hello; rm -rf /"

	// SAFE: exec.Command takes the program + args as SEPARATE strings. There is no
	// shell, so ';', '|', '$()' etc. are NOT interpreted — it's all one argument.
	out, _ := exec.Command("echo", userInput).Output()
	fmt.Printf("echo arg -> %q\n", strings.TrimSpace(string(out)))

	// DANGEROUS (never do this): handing a built string to a shell interprets the
	// metacharacters. Shown as a string only — do not exec sh -c with user input.
	danger := "sh -c \"echo " + userInput + "\""
	fmt.Println("would run:", danger)
}
```

**Output:**

```
echo arg -> "hello; rm -rf /"
would run: sh -c "echo hello; rm -rf /"
```

---

## 6. Path traversal

`🟢 easy` · *injection*

If you join user input to a base directory, `../../etc/passwd` can escape it. Defense: after joining, confirm the result is still **under** the base (via `filepath.Rel` + a `..` check). Go 1.24's `os.Root`/`os.OpenRoot` enforces this at the syscall level and is the modern choice for real file serving.

**Steps:**

1. `filepath.Join(base, userPath)` cleans the path (collapsing `..`).
2. `filepath.Rel(base, joined)` — if it starts with `..`, the path escaped → reject.
3. Legit sub-paths pass; traversal is rejected.

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// safeJoin joins userPath under base and rejects anything that escapes base. (Go
// 1.24's os.Root / os.OpenRoot enforces this at the syscall level — prefer it for
// real file serving.)
func safeJoin(base, userPath string) (string, bool) {
	p := filepath.Join(base, userPath)
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return p, true
}

func main() {
	base := "/srv/files"
	for _, in := range []string{"notes/a.txt", "../../etc/passwd", "a/../b.txt"} {
		p, ok := safeJoin(base, in)
		fmt.Printf("%-18q -> ok=%-5v %s\n", in, ok, p)
	}
}
```

**Output:**

```
"notes/a.txt"      -> ok=true  /srv/files/notes/a.txt
"../../etc/passwd" -> ok=false 
"a/../b.txt"       -> ok=true  /srv/files/b.txt
```

---

## 7. Limit the request body size

`🟢 easy` · *dos*

Without a cap, a client can stream a multi-gigabyte body and exhaust your memory — a cheap denial of service. `http.MaxBytesReader` wraps the body so reading past the limit returns an error (and sets a 413-friendly signal), which you turn into a clean rejection.

**Steps:**

1. `r.Body = http.MaxBytesReader(w, r.Body, 32)` caps the body at 32 bytes.
2. Reading a larger body fails → respond `413 Request Entity Too Large`.
3. A small body reads fine.

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

func main() {
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Cap the request body at 32 bytes. Reading past the cap errors, so a huge
		// upload can't exhaust memory (a cheap DoS defense).
		r.Body = http.MaxBytesReader(w, r.Body, 32)
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		fmt.Fprintln(w, "ok")
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	do := func(body string) {
		resp, _ := http.Post(srv.URL, "text/plain", strings.NewReader(body))
		fmt.Printf("body=%3d bytes -> %d\n", len(body), resp.StatusCode)
		resp.Body.Close()
	}
	do("small")
	do(strings.Repeat("x", 100))
}
```

**Output:**

```
body=  5 bytes -> 200
body=100 bytes -> 413
```

---

## 8. Reject unknown JSON fields

`🟢 easy` · *validation*

By default `json.Unmarshal` silently ignores fields it doesn't recognise — which lets an attacker try **overposting** (`{"is_admin":true}`) and hides client typos. `Decoder.DisallowUnknownFields()` makes an unexpected field a hard error.

**Steps:**

1. Use a `json.Decoder` (not `Unmarshal`) so you can configure it.
2. `dec.DisallowUnknownFields()` before `Decode`.
3. A payload with an extra field is rejected with a clear error.

```go
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CreateUser struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func decode(body string) error {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields() // reject fields we didn't define (typos, overposting)
	var u CreateUser
	return dec.Decode(&u)
}

func main() {
	fmt.Println("known fields:  ", decode(`{"name":"Alice","role":"user"}`))
	// An attacker adds "is_admin" hoping it binds to a privileged field.
	fmt.Println("unknown field: ", decode(`{"name":"Alice","is_admin":true}`))
}
```

**Output:**

```
known fields:   <nil>
unknown field:  json: unknown field "is_admin"
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
