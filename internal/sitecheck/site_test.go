package sitecheck_test

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const publicURL = "https://gonfff.github.io/jotmd/"

var assetPattern = regexp.MustCompile(`(?:href|src)="([^"#?]+\.(?:css|js|jpg|png|svg))"`)
var linkPattern = regexp.MustCompile(`href="([^"]+)"`)

func TestLandingPageServesLocalAssets(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	t.Cleanup(server.Close)

	body := get(t, server.URL+"/")
	assets := assetPattern.FindAllStringSubmatch(body, -1)
	if len(assets) == 0 {
		t.Fatal("landing page has no local assets")
	}
	for _, match := range assets {
		path := match[1]
		if strings.HasPrefix(path, "/") {
			t.Fatalf("root-relative asset breaks GitHub project pages: %s", path)
		}
		if strings.Contains(path, "://") {
			continue
		}
		get(t, server.URL+"/"+path)
	}
}

func TestLandingPageIncludesScreenshotSlideshow(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	t.Cleanup(server.Close)

	body := get(t, server.URL+"/")
	for _, image := range []string{
		"jotmd-browse.png",
		"jotmd-search.png",
		"jotmd-toc.png",
		"jotmd-commands.png",
		"jotmd-help.png",
	} {
		if !strings.Contains(body, `src="assets/`+image+`"`) {
			t.Errorf("landing page is missing %s", image)
		}
	}
	for _, removed := range []string{"jotmd-themes.png", "jotmd-memory.png"} {
		if strings.Contains(body, removed) {
			t.Errorf("landing page still includes removed screenshot %s", removed)
		}
	}
	if strings.Count(body, "data-slide-to=") != 5 {
		t.Error("screenshot slideshow must provide one control per slide")
	}
}

func TestLandingPageUsesVaultMapLayout(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	t.Cleanup(server.Close)

	body := get(t, server.URL+"/")
	for _, marker := range []string{
		`class="vault"`,
		`aria-label="Vault map"`,
		`id="readme"`,
		`id="browse"`,
		`id="commands"`,
		`id="cli"`,
		`id="memory"`,
		`id="install"`,
		`href="docs.html#cli"`,
		`href="docs.html#agent-memory"`,
		`jot-memory`,
		`<code>agent-memory/</code>`,
		`<kbd>a</kbd>`,
		`5 notes + 1 script`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("landing page is missing Vault Map marker %s", marker)
		}
	}
	if strings.Contains(body, `<a class="active"`) {
		t.Error("Vault Map selection must follow the URL target instead of staying on README")
	}
	checkLocalLinksAndAnchors(t, server, "/")
}

func TestDocumentationServesLocalLinksAndAnchors(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	t.Cleanup(server.Close)

	checkLocalLinksAndAnchors(t, server, "/docs.html")
}

func checkLocalLinksAndAnchors(t *testing.T, server *httptest.Server, page string) {
	t.Helper()
	body := get(t, server.URL+page)
	for _, match := range linkPattern.FindAllStringSubmatch(body, -1) {
		link, err := url.Parse(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if link.IsAbs() || link.Scheme != "" {
			continue
		}
		if strings.HasPrefix(link.Path, "/") {
			t.Fatalf("root-relative link breaks GitHub project pages: %s", link)
		}

		path := link.Path
		if path == "" {
			path = page
		} else {
			path = "/" + path
		}
		target := get(t, server.URL+path)
		if link.Fragment != "" && !strings.Contains(target, `id="`+link.Fragment+`"`) {
			t.Errorf("%s has no #%s target", link.Path, link.Fragment)
		}
	}
}

func TestDiscoveryFiles(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir("../../site")))
	t.Cleanup(server.Close)

	robots := get(t, server.URL+"/robots.txt")
	if !strings.Contains(robots, "Sitemap: "+publicURL+"sitemap.xml") {
		t.Error("robots.txt does not point to the public sitemap")
	}

	var sitemap struct {
		URLs []struct {
			Location string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(get(t, server.URL+"/sitemap.xml")), &sitemap); err != nil {
		t.Fatal(err)
	}
	locations := make(map[string]bool, len(sitemap.URLs))
	for _, entry := range sitemap.URLs {
		locations[entry.Location] = true
	}
	for _, expected := range []string{publicURL, publicURL + "docs.html"} {
		if !locations[expected] {
			t.Errorf("sitemap is missing %s", expected)
		}
	}

	index := get(t, server.URL+"/llms.txt")
	if !strings.HasPrefix(index, "# JotMD\n") {
		t.Error("llms.txt must start with the project H1")
	}
	if !strings.Contains(index, "]("+publicURL+"docs.html)") {
		t.Error("llms.txt does not link to the public documentation")
	}

	for _, heading := range []string{"## Installation", "## CLI", "## Agent memory"} {
		if !strings.Contains(index, heading) {
			t.Errorf("llms.txt is missing %s", heading)
		}
	}

	for _, page := range []string{"/", "/docs.html"} {
		body := get(t, server.URL+page)
		if !strings.Contains(body, `rel="alternate" type="text/markdown" href="llms.txt"`) {
			t.Errorf("%s does not advertise llms.txt", page)
		}
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", url, response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
