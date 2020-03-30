# RSS Reader Redmine Bot
This is a simple implementation of a RSS reader bot for Redmine written in golang.
( for redmine in private network)

## How To Config
First, clone this project then build code.
```
# clone ....
# cd go-rss-reader-redmine-bot
# go get -d
# go build rss_reader_redmine_bot.go
```

Next edit redmine_rss.conf.

Example
```
# This tool logging the entry id that already sended.
# comp_file_path define the id logging file path.
comp_file_path = "./fn/"

# polling projects interval (seconds)
polling_interval = 60


# Setting projects that you want to poll changes and notify.
[[projects]]
# Setting the project id. No duplication.
id = "sample-project01_activity"
# rss(atom) url
url = "https://redmine.org/projects/sample-project-01/activity.atom?key=xxxxxx"
  [projects.slack]
  # bot_name
  bot_name = "bot_name"
  # The maximum number of lines to send slack(content only)
  max_lines = 3
  # webhook_url
  webhook_url = "https://hooks.slack.com/services/xxxxxxxx"
  # channel(post to) and bot's icon
  channel = "general"
  icon = "pig"

[[projects]]
.....
```

Dryrun
At the first time, this script poll and send the all available entries.
So please try dryrun to loggin current entry ( exec with --dryrun=true flag).
```
# ./rss_reader_redmine_bot --dryrun --config.file=./redmine_rss.conf
```

Setting cron to exec this script (without --dryrun flag)

# Contributing
Please submit PRs.
