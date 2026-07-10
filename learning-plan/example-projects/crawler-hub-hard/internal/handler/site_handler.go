package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// SiteHandler serves a built-in, deterministic mini-site so the crawler has
// something OFFLINE and REPRODUCIBLE to crawl. The link graph has cycles and
// back-links, which is what proves the crawler's dedup actually works — a naive
// crawler would loop forever on it.
type SiteHandler struct {
	pages int // number of pages in the generated site (ids 0 .. pages-1)
}

func NewSiteHandler(pages int) *SiteHandler {
	if pages < 1 {
		pages = 1
	}
	return &SiteHandler{pages: pages}
}

// Page serves GET /site/{page}: a tiny HTML page whose body links to a handful
// of other pages by id. Deterministic — the neighbours are pure arithmetic, no
// randomness — so the same crawl always visits the same graph.
func (s *SiteHandler) Page(w http.ResponseWriter, r *http.Request) {
	// PathValue reads a wildcard captured by the route pattern ("/site/{page}").
	id, err := strconv.Atoi(r.PathValue("page"))
	if err != nil || id < 0 || id >= s.pages {
		http.NotFound(w, r) // out-of-range id → 404, same as a real dead link
		return
	}

	// strings.Builder assembles the HTML without allocating a new string per +.
	var b strings.Builder
	fmt.Fprintf(&b, "<html><body><h1>Page %d</h1>", id)
	for _, nb := range s.neighbors(id) {
		// Relative hrefs on purpose: the crawler must ResolveReference them.
		fmt.Fprintf(&b, `<a href="/site/%d">page %d</a> `, nb, nb)
	}
	b.WriteString("</body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, b.String())
}

// neighbors returns the page ids that page `id` links to. The mix of forward
// jumps, a doubling, and a backward wrap produces cross-links and CYCLES (e.g.
// page 0 links forward, some later page wraps back to 0), giving the crawler's
// dedup something real to do.
func (s *SiteHandler) neighbors(id int) []int {
	n := s.pages
	if n <= 1 {
		return nil
	}
	cand := []int{
		(id + 1) % n,
		(id + 3) % n,
		(id + 7) % n,
		(id*2 + 1) % n,
		(id + n - 1) % n, // backward wrap: guarantees back-links / cycles
	}
	seen := map[int]struct{}{id: {}} // exclude self-links
	out := make([]int, 0, len(cand))
	for _, c := range cand {
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
