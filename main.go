package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Post struct {
	ID       string
	Title    string
	Subtitle string
	Path     string
	Date     time.Time
	IsDraft  bool
	HTMLBody template.HTML
}

// URL is the site-absolute path to the rendered post page.
func (p Post) URL() string {
	return basePath + "/post/" + p.ID + ".html"
}

// RenderData is the top-level value passed to the master template: Posts for the
// index, Post for a single article.
type RenderData struct {
	Posts []Post
	Post  *Post
}

var devMode bool

// postsDir is where the .md posts live: the bugo folder of the Obsidian notes
// vault (BUGO_POSTS overrides it; a leading ~ is expanded). basePath is the URL prefix of the published site
// (BUGO_BASE, e.g. "/bugo" for a GitHub Pages project site); empty in dev.
var (
	postsDir = expandHome(envOr("BUGO_POSTS", "~/Notes/bugo/posts"))
	basePath = strings.TrimRight(os.Getenv("BUGO_BASE"), "/")
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func expandHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func main() {
	args := os.Args
	if len(args) > 1 {
		switch args[1] {
		case "dev":
			devMode = true
			basePath = ""
			fs := http.FileServer(http.Dir("public/"))
			http.HandleFunc("/", indexHandler)
			http.Handle("/static/", fs)
			http.HandleFunc("/post/{id}", postHandler)
			http.HandleFunc("/dev/sse", sseHandler)
			if err := startWatcher("templates", "public", postsDir); err != nil {
				log.Printf("watcher error: %v", err)
			}
			log.Printf("Server on http://localhost:3000")
			log.Fatal(http.ListenAndServe(":3000", nil))

		case "pub":
			if err := os.MkdirAll("public/post", 0755); err != nil {
				log.Fatal(err)
			}
			posts, err := loadPosts()
			if err != nil {
				log.Fatalf("load posts: %v", err)
			}
			for _, post := range posts {
				pf, err := os.Create("public/post/" + post.ID + ".html")
				if err != nil {
					log.Fatal(err)
				}
				renderPost(pf, post)
				pf.Close()
			}
			pf, err := os.Create("public/index.html")
			if err != nil {
				log.Fatal(err)
			}
			renderIndex(pf, posts)
			pf.Close()
		}
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	posts, err := loadPosts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderIndex(w, posts)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	id = strings.TrimSuffix(id, ".html")
	post, err := loadPost(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if post == nil {
		http.NotFound(w, r)
		return
	}
	renderPost(w, *post)
}

func parseTemplates(files ...string) *template.Template {
	if devMode {
		files = append(files, "templates/hot_reload.html")
	}
	tmpl := template.New("master.html").Funcs(template.FuncMap{
		"formatDate": formatDate,
		"base":       func() string { return basePath },
	})
	return template.Must(tmpl.ParseFiles(files...))
}

func renderPost(w io.Writer, post Post) {
	data := RenderData{Post: &post}
	tmpl := parseTemplates("templates/master.html", "templates/post.html")
	tmpl.ExecuteTemplate(w, "master.html", data)
}

func renderIndex(w io.Writer, posts []Post) {
	data := RenderData{Posts: posts}
	tmpl := parseTemplates("templates/master.html", "templates/index.html")
	tmpl.ExecuteTemplate(w, "master.html", data)
}

// parsePost splits a raw .md file into front matter and body and builds a Post.
// The caller fills in ID.
func parsePost(content []byte) (Post, error) {
	parts := strings.SplitN(string(content), "+++", 3)
	if len(parts) != 3 {
		return Post{}, fmt.Errorf("invalid front matter: expected opening and closing +++ delimiters")
	}
	// parts[0] is empty because the file starts with +++.
	post := parseFrontMatter(parts[1])
	post.HTMLBody = template.HTML(parseMD(parts[2]))
	return post, nil
}

// loadPost loads a single post by id from <postsDir>/<id>.md (used by the dev
// server). Returns (nil, nil) when not found.
func loadPost(id string) (*Post, error) {
	content, err := os.ReadFile(filepath.Join(postsDir, id+".md"))
	if err != nil {
		return nil, nil // not found
	}
	post, err := parsePost(content)
	if err != nil {
		return nil, err
	}
	post.ID = id
	return &post, nil
}

// loadPosts reads every <postsDir>/<id>.md, skips drafts, and sorts newest first.
func loadPosts() ([]Post, error) {
	entries, err := os.ReadDir(postsDir)
	if err != nil {
		return nil, err
	}
	var posts []Post
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(postsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		post, err := parsePost(content)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(postsDir, e.Name()), err)
		}
		post.ID = strings.TrimSuffix(e.Name(), ".md")
		if !post.IsDraft {
			posts = append(posts, post)
		}
	}
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date.After(posts[j].Date)
	})
	return posts, nil
}

func formatDate(t time.Time) string {
	return t.Format("02 Jan 2006")
}
