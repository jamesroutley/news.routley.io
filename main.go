package main

import (
	"context"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"text/template"
	"time"

	"github.com/mmcdole/gofeed"
)

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
	}

	// Show up to 30 days of posts
	relevantDuration = 30 * 24 * time.Hour

	outputDir  = "docs" // So we can host the site on GitHub Pages
	outputFile = "index.html"
)

func main() {
	if err := run(context.TODO()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	posts := getPosts(ctx, feeds)

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return err
	}

	f, err := os.Create(path.Join(outputDir, outputFile))
	if err != nil {
		return err
	}
	defer f.Close()

	if err := executeTemplate(f, posts); err != nil {
		return err
	}

	return nil
}

// getPosts returns all posts from all feeds from the last `relevantDuration`
// time period. Posts are sorted chronologically descending.
func getPosts(ctx context.Context, feeds []string) []*Post {
	var posts []*Post
	for _, feed := range feeds {
		parser := gofeed.NewParser()
		feed, err := parser.ParseURLWithContext(feed, ctx)
		if err != nil {
			log.Println(err)
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
			posts = append(posts, &Post{
				Link:      item.Link,
				Title:     item.Title,
				Published: *published,
				Host:      parsedLink.Host,
			})
		}
	}

	// Sort items chronologically descending
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Published.After(posts[j].Published)
	})

	return posts
}

func executeTemplate(writer io.Writer, posts []*Post) error {
	htmlTemplate := `
<!DOCTYPE html>
<html>
	<head>
		<meta charset="utf-8">
		<meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>James Routley | Feed</title>
		<link type="text/css" rel="stylesheet" href="css/styles.css">
	</head>
	<body>
		<h1>News</h1>

		<ol>
			{{ range . }}<li><a href="{{ .Link }}">{{ .Title }}</a> ({{ .Host }})</li>
			{{ end }}
		</ol>
	</body>
</html>
`

	tmpl, err := template.New("webpage").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(writer, posts); err != nil {
		return err
	}

	return nil
}
