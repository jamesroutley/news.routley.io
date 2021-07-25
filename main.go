package main

import (
	"context"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/mmcdole/gofeed"
)

var (
	wg sync.WaitGroup
)

type TemplateData struct {
	Posts []*Post
}

type Post struct {
	Link      string
	Title     string
	Published time.Time
	Host      string
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
		"https://samstarling.co.uk/weeknotes/index.xml",
		"https://macwright.com/rss.xml",
	}

	// Show up to 60 days of posts
	relevantDuration = 60 * 24 * time.Hour

	outputDir  = "docs" // So we can host the site on GitHub Pages
	outputFile = "index.html"

	// Error out if fetching feeds takes longer than a minute
	timeout = time.Minute
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	posts := getAllPosts(ctx, feeds)

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return err
	}

	f, err := os.Create(path.Join(outputDir, outputFile))
	if err != nil {
		return err
	}
	defer f.Close()

	templateData := &TemplateData{
		Posts: posts,
	}

	if err := executeTemplate(f, templateData); err != nil {
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
	go func() {
		for post := range postChan {
			posts = append(posts, post)
		}
	}()

	wg.Wait()
	close(postChan)

	// Sort items chronologically descending
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Published.After(posts[j].Published)
	})

	return posts
}

func getPosts(ctx context.Context, feedURL string, posts chan *Post) {
	defer wg.Done()
	parser := gofeed.NewParser()
	feed, err := parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		log.Println(err)
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
		post := &Post{
			Link:      item.Link,
			Title:     item.Title,
			Published: *published,
			Host:      parsedLink.Host,
		}
		posts <- post
	}
}

func executeTemplate(writer io.Writer, templateData *TemplateData) error {
	htmlTemplate := `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="utf-8">
		<meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>James Routley | Feed</title>
		<style>
			@import url("https://fonts.googleapis.com/css2?family=Nanum+Myeongjo&display=swap");

			body {
				font-family: "Nanum Myeongjo", serif;
				line-height: 1.7;
				max-width: 600px;
				margin: 50px auto 50px;
				padding: 0 12px 0;
				height: 100%;
			}

			li {
				padding-bottom: 16px;
			}
		</style>
	</head>
	<body>
		<h1>News</h1>

		<ol>
			{{ range .Posts }}<li><a href="{{ .Link }}">{{ .Title }}</a> ({{ .Host }})</li>
			{{ end }}
		</ol>

		<footer>
			<p><a href="https://github.com/jamesroutley/news.routley.io">What is this?</a></p>
		</footer>
	</body>
</html>
`

	tmpl, err := template.New("webpage").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(writer, templateData); err != nil {
		return err
	}

	return nil
}
