// Copyright 2017 phcurtis ghrepo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Main generates GitHubReposReportSummary see corresponding function for more details.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type dataStruct struct {
	Name            string    `json:"name"`
	PushedAt        time.Time `json:"pushed_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	WatchersCount   int       `json:"watchers_count"`
	OpenIssuesCount int       `json:"open_issues_count"`
}

const version = "0.10"

func (d dataStruct) String() string {
	return fmt.Sprintf("[Name:%s UpdatedAt:%v PushedAt:%v WatchersCount:%d OpenIssuesCount:%d]",
		d.Name, d.UpdatedAt, d.PushedAt, d.WatchersCount, d.OpenIssuesCount)
}

type interface2 interface {
	Title() string
	Name(int) string
	Field(int) string
	sort.Interface
}

type ghStruct struct {
	title   string
	sortasc bool
	data    []dataStruct
}

// byUpdateAt stuff  for sort.Sort
type byUpdatedAt ghStruct

func (a byUpdatedAt) Title() string      { return a.title }
func (a byUpdatedAt) Name(i int) string  { return a.data[i].Name }
func (a byUpdatedAt) Field(i int) string { return fmt.Sprintf("%v", a.data[i].UpdatedAt) }
func (a byUpdatedAt) Len() int           { return len(a.data) }
func (a byUpdatedAt) Swap(i, j int)      { a.data[i], a.data[j] = a.data[j], a.data[i] }
func (a byUpdatedAt) Less(i, j int) bool {
	if a.sortasc {
		return a.data[i].UpdatedAt.Before(a.data[j].UpdatedAt)
	}
	return a.data[i].UpdatedAt.After(a.data[j].UpdatedAt)
}

// byPushedAt stuff for sort.Sort
type byPushedAt ghStruct

func (a byPushedAt) Title() string      { return a.title }
func (a byPushedAt) Name(i int) string  { return a.data[i].Name }
func (a byPushedAt) Field(i int) string { return fmt.Sprintf("%v", a.data[i].PushedAt) }
func (a byPushedAt) Len() int           { return len(a.data) }
func (a byPushedAt) Swap(i, j int)      { a.data[i], a.data[j] = a.data[j], a.data[i] }
func (a byPushedAt) Less(i, j int) bool {
	if a.sortasc {
		return a.data[i].PushedAt.Before(a.data[j].PushedAt)
	}
	return a.data[i].PushedAt.After(a.data[j].PushedAt)
}

func getData(urlname string) ([]dataStruct, error) {
	var err error
	var req *http.Request
	var res *http.Response
	var body []byte
	var data, totData []dataStruct
	page := 0
	for {
		page++
		pagination := fmt.Sprintf("?page=%d", page)

		if req, err = http.NewRequest("GET", urlname+pagination, nil); err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", `application/json; charset=utf-8`)
		if res, err = http.DefaultClient.Do(req); err != nil {
			return nil, err
		}
		defer func() { _ = res.Body.Close() }()

		if body, err = ioutil.ReadAll(res.Body); err != nil {
			return nil, err
		}

		//fmt.Printf("%s\n", strings.Join(strings.Split(string(body), ","), "\n"))
		if err = json.Unmarshal(body, &data); err != nil {
			const rateErr = "API rate limit exceeded"
			if strings.Contains(string(body), rateErr) {
				return nil, fmt.Errorf("json.Unmarshal failed likely because of:%q jsonErr:%q", rateErr, err)
			}
			return nil, err
		}

		totData = append(totData, data...)

		link := res.Header.Get("Link")
		//fmt.Printf("Link:%v\n", res.Header.Get("Link"))

		if !strings.Contains(link, `rel="next"`) {
			return totData, nil
		}
	}
}

type sortType uint16

// sortType values
const (
	sbyUpdatedAt sortType = 1 << iota
	sbyPushedAt
	sascending
	sdefault = sbyUpdatedAt
)

// gitHubReposReportSummary - generates a summary for a given github url that
// includes: totOpenIssues, mostWatchersRepo and a sorted list of repos by sortType
// - urlname - name of github url for getting repos info
// - writer  - io.Writer to generate output too.
// - sorttype - see sortType values
func gitHubReposReportSummary(urlname string, writer io.Writer, sortby sortType) error {
	reportName := "GitHubReposReportSummary"

	data, err := getData(urlname)
	if err != nil {
		return err
	}

	totOpenIssues := 0
	maxWatchers := 0
	maxWatchersName := "<NONE>"
	for _, v := range data {
		totOpenIssues += v.OpenIssuesCount
		if v.WatchersCount < 0 {
			return fmt.Errorf("WatchersCount is negative! %v", v.String())
		}
		if v.WatchersCount > maxWatchers {
			maxWatchersName = v.Name
			maxWatchers = v.WatchersCount
		} else if v.WatchersCount > 0 && v.WatchersCount == maxWatchers {
			maxWatchersName += "," + v.Name
		}
	}

	fmt.Fprintf(writer, "%s:\nPublic accessible info for %s\n", reportName, urlname)
	fmt.Fprintf(writer, "totOpenIssues:%d mostWatchersRepo:%s [maxWatchers:%d]\n",
		totOpenIssues, maxWatchersName, maxWatchers)

	var bdata interface2
	asc := sortby&sascending > 0
	asctxt := "ascending"
	if !asc {
		asctxt = "descending"
	}
	switch {
	case sortby&sbyPushedAt > 0:
		bdata = byPushedAt{"byPushedAt " + asctxt, asc, data}
	default:
		fallthrough
	case sortby&sbyUpdatedAt > 0:
		bdata = byUpdatedAt{"byUpdatedAt " + asctxt, asc, data}
	}
	sort.Sort(bdata)
	fmt.Fprintf(writer, "Repos [%d] sorted by %s:\n", bdata.Len(), bdata.Title())
	for i := 0; i < bdata.Len(); i++ {
		fmt.Fprintf(writer, "i:%2d %v %s\n", i, bdata.Field(i), bdata.Name(i))
	}
	fmt.Fprintf(writer, "<endOfReport: %s>\n", reportName)

	return nil
}

type flagsStruct struct {
	showVersion bool
	verbose     int
	ghurl       string
	ascending   bool
	bypushedat  bool
}

// example of organization github api repos url : "https://api.github.com/orgs/gorilla/repos"
// example of users        github api repos url: "https://api.github.com/users/phcurtis/repos"

var flags = flagsStruct{}

const ghurlDef = "https://api.github.com/users/phcurtis/repos"

func init() {
	flag.StringVar(&flags.ghurl, "ghurl", ghurlDef, "github url for getting repos info")
	flag.BoolVar(&flags.showVersion, "version", false, "show version")
	flag.IntVar(&flags.verbose, "verbose", 0, "verbose level")
	flag.BoolVar(&flags.ascending, "ascending", false, "sort ascending")
	flag.BoolVar(&flags.bypushedat, "bypushedat", false, "sort bypushedat field")
}

func main() {
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unrecognized %v\nUsage of ./%s:\n",
			flag.Args(), filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(1)
	}
	if flags.verbose > 0 {
		fmt.Printf("%v version:%s\n", os.Args, version)
	}
	if flags.showVersion {
		fmt.Printf("./%s version=%s\n", filepath.Base(os.Args[0]), version)
	}
	var stype sortType
	if flags.ascending {
		stype = sascending
	}
	if flags.bypushedat {
		stype |= sbyPushedAt
	}

	err := gitHubReposReportSummary(flags.ghurl, os.Stdout, stype)
	if err != nil {
		log.Fatalf("%s: err:%v\n", os.Args, err)
	}
}
