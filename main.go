package main

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cixtor/readability"
	"github.com/mmcdole/gofeed"
)

var (
	wg              sync.WaitGroup
	reader          = readability.New()
	nonAlphanumeric = regexp.MustCompile(`([^\w\d])`)
)

//go:embed index.template.html
var indexTmpl string

//go:embed post.template.html
var postTmpl string

//go:embed styles.css
var styles string

type Post struct {
	Link      string
	Title     string
	Published time.Time
	Host      string
	Content   template.HTML
	Filename  string
}

var (
	feeds = []string{
		"https://themargins.substack.com/feed.xml",
		"https://jvns.ca/atom.xml",
		"https://joy.recurse.com/feed.atom",
		"https://danluu.com/atom.xml",
		"https://blog.veitheller.de/feed.rss",
		"https://twobithistory.org/feed.xml",
		"https://rachelbythebay.com/w/atom.xml",
		"https://scattered-thoughts.net/rss.xml",

		// My mum's food blog 💖
		"https://gochugarugirl.com/feed/",

		"https://commoncog.com/blog/rss/",
		"https://highgrowthengineering.substack.com/feed",
		"http://tonsky.me/blog/atom.xml",
		"https://www.benkuhn.net/index.xml",

		"https://hnrss.org/frontpage?points=50",
		"https://solar.lowtechmagazine.com/feeds/all-en.atom.xml",
		"https://www.slowernews.com/rss.xml",
		"https://macwright.com/rss.xml",
		"https://mikehudack.substack.com/feed",
		"https://www.wildlondon.org.uk/blog/all/rss.xml",
		"https://blaggregator.recurse.com/atom.xml?token=4c4c4e40044244aab4a36e681dfb8fb0",

		"https://blog.golang.org/feed.atom?format=xml",
		"https://anewsletter.alisoneroman.com/feed",

		"https://routley.io/reserialised/great-expectations/2022-08-24/index.xml",
	}

	// Show up to 60 days of posts
	// TODO
	relevantDuration = 7 * 24 * time.Hour

	outputDir  = "docs" // So we can host the site on GitHub Pages
	outputFile = "index.html"
	logFile    = path.Join(outputDir, "log.txt")

	// Error out if fetching feeds takes longer than a minute
	timeout = time.Minute
)

var blocklist = map[string]bool{
	"www.metafilter.com": true,
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return err
	}

	logF, err := os.Create(logFile)
	if err != nil {
		return err
	}
	log.SetOutput(logF)

	posts := getAllPosts(ctx, feeds)

	start := time.Now()
	defer func() {
		log.Printf("Templated %d posts, took %s", len(posts), time.Since(start))
	}()

	if err := os.RemoveAll(path.Join(outputDir, "posts")); err != nil {
		return err
	}

	if err := os.MkdirAll(path.Join(outputDir, "posts"), 0700); err != nil {
		return err
	}

	for _, post := range posts {
		if string(post.Content) == "" {
			log.Printf("Skipping writing post, no content: %s", post.Link)
			continue
		}
		f, err := os.Create(path.Join(outputDir, "posts", post.Filename))
		if err != nil {
			return err
		}

		tmpl, err := template.New("post").Parse(postTmpl)
		if err != nil {
			return err
		}
		data := map[string]interface{}{
			"Styles":   template.CSS(styles),
			"Content":  post.Content,
			"Original": post.Link,
			"Title":    post.Title,
		}
		if err := tmpl.Execute(f, data); err != nil {
			return err
		}

		f.Close()
	}

	f, err := os.Create(path.Join(outputDir, outputFile))
	if err != nil {
		return err
	}
	defer f.Close()

	data := map[string]interface{}{
		"Posts":  posts,
		"Styles": template.CSS(styles),
	}
	if err := executeTemplate(f, data); err != nil {
		return err
	}

	return nil
}

// getAllPosts returns all posts from all feeds from the last `relevantDuration`
// time period. Posts are sorted chronologically descending.
func getAllPosts(ctx context.Context, feeds []string) []*Post {
	postChan := make(chan *Post)

	wg.Add(len(feeds))
	for _, feed := range feeds {
		go getPosts(ctx, feed, postChan)
	}

	var posts []*Post
	seen := map[string]bool{}
	go func() {
		for post := range postChan {
			if seen[post.Link] {
				continue
			}
			posts = append(posts, post)
			seen[post.Link] = true
		}
	}()

	wg.Wait()
	close(postChan)

	// Sort items chronologically descending
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].Published.Equal(posts[j].Published) {
			return posts[i].Title < posts[j].Title
		}
		return posts[i].Published.After(posts[j].Published)
	})

	return posts
}

func getPosts(ctx context.Context, feedURL string, posts chan *Post) {
	start := time.Now()
	defer func() {
		log.Printf("Fetched posts from %s, took %s", feedURL, time.Since(start))
	}()
	defer wg.Done()
	parser := gofeed.NewParser()
	feed, err := parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		log.Println(fmt.Errorf("error parsing %s: %w", feedURL, err))
		return
	}

	for _, item := range feed.Items {
		published := item.PublishedParsed
		if published == nil {
			published = item.UpdatedParsed
		}
		if published.Before(time.Now().Add(-relevantDuration)) {
			continue
		}
		parsedLink, err := url.Parse(item.Link)
		if err != nil {
			log.Println(err)
		}

		if blocklist[parsedLink.Host] {
			continue
		}

		post := &Post{
			Link:      item.Link,
			Title:     item.Title,
			Published: *published,
			Host:      parsedLink.Host,
			Content:   template.HTML(item.Content),
			Filename:  titleToFilename(item.Title),
		}

		// If content isn't available from RSS, try to pull it from the webpage
		// itself
		if post.Content == "" {
			req, err := http.NewRequestWithContext(ctx, "GET", item.Link, nil)
			if err != nil {
				log.Println(err)
				continue
			}
			req.Header["User-Agent"] = []string{
				"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Println(err)
				continue
			}

			// Don't try an parse non-HTML
			contentType := rsp.Header.Get("content-type")
			if contentType != "" && !strings.HasPrefix(contentType, "text/html") {
				log.Printf("Not HTML: %s\n", item.Link)
				continue
			}

			article, err := reader.Parse(rsp.Body, item.Link)
			if err != nil {
				log.Println(err)
				continue
			}

			post.Content = template.HTML(article.Content)

			if post.Content == "" {
				log.Printf("Content still empty after HTML reader: %s", item.Link)
			}

			rsp.Body.Close()
		}

		posts <- post
	}
}

func titleToFilename(s string) string {
	s = nonAlphanumeric.ReplaceAllLiteralString(s, " ")
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	s = strings.Join(fields, "-") + ".html"
	s = strings.TrimPrefix(s, "show-hn-")
	s = strings.TrimPrefix(s, "ask-hn-")
	return s
}

func executeTemplate(writer io.Writer, templateData map[string]interface{}) error {
	tmpl, err := template.New("index").Parse(indexTmpl)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(writer, templateData); err != nil {
		return err
	}

	return nil
}
