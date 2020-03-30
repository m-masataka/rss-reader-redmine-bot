package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"sync"
	"context"
	"io"
	"strings"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/BurntSushi/toml"
	"github.com/mmcdole/gofeed/atom"
	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/lunny/html2md"
)

type Config struct {
	CompFilePath    string `toml:"comp_file_path"`
	PollingInterval int    `toml:"polling_interval"`
	Projects []Project
}

type Project struct {
	Url   string `toml:"url"`
	Id    string `toml:"id"`
	Slack Slack  `toml:"slack"`
}

type Slack struct {
	WebhookUrl string `toml:"webhook_url"`
	Channel    string `toml:"channel"`
	BotName    string `toml:"bot_name"`
	Icon       string `toml:"icon"`
	MaxLines   int    `toml:"max_lines"`
}

var (
	logger log.Logger
	configFile = kingpin.Flag("config.file", "Config file path").Required().String()
	redmineProxy = kingpin.Flag("redmine.proxy", "Set Proxy to redmine server").Default("").String()
	slackProxy = kingpin.Flag("slack.proxy", "Set Proxy to slack api").Default("").String()
	dryrun = kingpin.Flag("dryrun", "Set Proxy to slack api").Bool()

	wg sync.WaitGroup
)


func main() {
	os.Exit(run())
}

func run() int{
	// Setting logger
	w := log.NewSyncWriter(os.Stdout)
	logger = log.NewLogfmtLogger(w)
	format := log.TimestampFormat(
		func() time.Time { return time.Now().UTC() },
		time.RFC3339Nano,
        )
	logger = log.With(logger, "timestamp", format, "caller", log.DefaultCaller)
	logger = level.NewFilter(logger, level.AllowInfo())
        pid := os.Getpid()
	level.Info(logger).Log("msg", "Start rss reader for redmine", "pid", pid)

	// parse args
	kingpin.Parse()
	// parse config
	var conf Config
	_, err := toml.DecodeFile(*configFile, &conf)
	if err != nil {
		level.Error(logger).Log("msg", "Config parse error", "error", err)
		return 1
	}

	
	var term = make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		wg.Wait()
	}()

	// Start polling rss
	wg.Add(1)
	go pollProjects(ctx, conf.PollingInterval, conf.Projects, conf.CompFilePath)

	for {
		select {
			case <-term:
				level.Info(logger).Log("msg", "Ended", "pid", pid)
				return 0
		}
	}
}

func pollProjects(ctx context.Context, interval int, pjs []Project, path string) {
	defer wg.Done()
	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
	loop:
		for {
			select {
			case <-ticker.C:
				for _, project := range pjs {
					wg.Add(1)
					go pollProject(project, path)
				}
			case <-ctx.Done():
				level.Info(logger).Log("msg", "poll projects routine ended.")
				break loop
			}
		}
	}()
}


func pollProject (p Project, path string){
	defer wg.Done()
	url := p.Url
	bkFile := path + p.Id + ".log"

	currentId, err := loadCurrentId(bkFile)
	if err != nil {
		level.Error(logger).Log("error", err)
	}

	feed, err := parseBodyByUrl(url)
	if err != nil {
		level.Error(logger).Log("error", err)
	}
	var entryList []atom.Entry
	for _, entry := range feed.Entries {
		if currentId == entry.ID {
			break
		}
		entryList = append(entryList, *entry)
	}
	for i := len(entryList) -1; i >= 0 ; i-- {
		if !(*dryrun) {
			err = sendSlack(p.Slack, entryList[i])
			if err != nil {
				level.Error(logger).Log("error", err)
				break
			}
		}
			
		err = logEntryId(bkFile, entryList[i].ID)
		if err != nil {
			level.Error(logger).Log("error", err)
		}
	}
}

func sendSlack(s Slack, e atom.Entry) (error) {
	attachment := slack.Attachment {}
	authors := ""
	for _, person := range e.Authors {
		authors = authors + person.Name
	}
	content := countRune(html2md.Convert(e.Content.Value), '\n', s.MaxLines)
	attachment.AddField(slack.Field { Title: "Authors", Value: authors})
	attachment.AddField(slack.Field { Title: "Content", Value: content})
	payload := slack.Payload {
		Text: "<" + e.Links[0].Href + "|" + e.Title + ">",
		Username: s.BotName,
		Channel: "#" + s.Channel,
		IconEmoji: ":" + s.Icon + ":",
		Attachments: []slack.Attachment{attachment},
		Markdown: true,
	}
	err := slack.Send(s.WebhookUrl, *slackProxy, payload)
	if len(err) > 0 {
		return fmt.Errorf("error: %s\n", err)
	}
	level.Info(logger).Log("msg", "Entity Sended", "Title", e.Title)
	return nil
}

func loadCurrentId(path string) (string, error) {
	line := ""
	f, err := os.OpenFile(path, os.O_CREATE, 0666)
	if err != nil {
	    return line, err
	}
	defer f.Close()
	byteSize := int64(500)
	f.Seek(-1*(byteSize+1), 2)
	b := make([]byte, (byteSize + 1))
	for {
		_, err := f.Read(b)
		if err != nil {
			if err != io.EOF {
				return line, nil
			}
			break
		}
	}
	line = string(b)
	id := strings.Split(strings.Replace(line, "\r\n", "\n", -1), "\n")
	if len(id) < 2 {
		return line, nil
	}
	return id[len(id) - 2], nil
}

func logEntryId(path string, id string) (error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
	    return err
	}
	defer f.Close()
	fmt.Fprintln(f, id) 
	return nil
}

func parseBodyByUrl(url string) (*atom.Feed, error) {
	ap := atom.Parser{}
	client := new(http.Client)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		defer func() {
			ce := resp.Body.Close()
			if ce != nil {
				err = ce
			}
		}()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ERROR: HTTP Status Code %d", resp.StatusCode)
	}	
		
	return ap.Parse(resp.Body)
}

func countRune(s string, r rune, m int) string {
	count := 0
	res := ""
	for _, c := range s {
		if c == r {
			count++
		}
		res = res + string(c)
		if count > m {
			return res + "..."
		}
	}
	return res
}
