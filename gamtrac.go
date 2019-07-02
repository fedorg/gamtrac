package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"gamtrac/api"
	"gamtrac/rules"
	"gamtrac/scanner"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deckarep/golang-set"
	"github.com/r3labs/diff"
)

type HashDigest = api.HashDigest
type FileError = api.FileError

type MountedPath struct {
	Destination string
	MountedAt   string
	Mounted     bool
}

func (p MountedPath) Unmount() error {
	if p.Mounted {
		out, err := scanner.UnmountShare(p.MountedAt)
		if err != nil {
			fmt.Printf("cannot unmount path: %v\n%v\n", err, string(out))
			return err
		}
	}
	return nil
}

type AnnotItem struct {
	path     MountedPath
	fileInfo os.FileInfo
	rules    []rules.RuleMatcher
	ruleIDs  []int
	queuedAt time.Time
}

var test_rules = []string{
	"<дата>_<проект>_<методика>_<вид данных>_<наименование образца>_<комментарий>",
	"R:\\DAR\\LAM\\Screening group\\<Заказчик>\\1_Результаты, протоколы, отчеты\\<Измеряемый параметр>_<Метод анализа>\\<Проект>\\"}

func computeHash(path string) (*HashDigest, error) {
	if !(os.Getenv("GAMTRAC_HASH_FILE_CONTENTS") > "0") {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 4*1024*1024) // larger transfers are faster
	h := sha256.New()
	_, err = io.Copy(h, reader)
	if err != nil {
		return nil, err
	}
	digest := &HashDigest{Value: h.Sum(nil), Algorithm: "sha256"}
	return digest, nil
}

func NewFileError(err error) FileError {
	fmt.Fprintln(os.Stderr, err)
	return FileError{
		// Filename:  filename,
		Error:     err,
		CreatedAt: time.Now(),
	}
}

func processFile(inputs <-chan AnnotItem, output chan<- api.AnnotResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for input := range inputs {
		errors := []FileError{}

		destination := input.path.Destination
		mountedAt := input.path.MountedAt
		info := input.fileInfo
		// fmt.Println("Processing file: ", mountedAt)
		ruleID := -1 // TODO: somehow get rid of this
		matches := rules.MatchAllRules(destination, input.rules)
		i := rules.FindBestRuleIndex(matches)
		ruleResult := api.PathTagsResult{
			Path:   destination,
			RuleID: -1,
			Values: map[string]string{},
		}
		if i != -1 {
			m := matches[i]
			ruleResult = api.PathTagsResult{
				Path:   destination,
				RuleID: input.ruleIDs[i],
				Values: m.AsMap(),
			}
		}
		owner, err := scanner.GetFileOwnerUID(mountedAt)
		if err != nil {
			errors = append(errors, NewFileError(err))
		}
		var hash *HashDigest = nil
		if !info.IsDir() {
			hash, err = computeHash(mountedAt)
			if err != nil {
				errors = append(errors, NewFileError(err))
			}
		}
		// TODO: this should be just a notification for other microservices to start parsing
		ret := api.FilePropsResult{
			Path:        destination,
			RuleID:      ruleID,
			MountDir:    mountedAt,
			Size:        info.Size(),
			Mode:        info.Mode(),
			ModTime:     info.ModTime(),
			QueuedAt:    input.queuedAt,
			ProcessedAt: time.Now(),
			IsDir:       info.IsDir(),
			OwnerUID:    owner,
			Hash:        hash,
			Errors:      errors,
		}
		// fmt.Println("Finished processing file: ", mountedAt)
		output <- &ret
		output <- &ruleResult
	}
}

func collectResults(annots <-chan api.AnnotResult, out chan<- map[string][]api.AnnotResult) {
	// TODO: make this less ugly
	ret := map[string][]api.AnnotResult{}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	m := &sync.Mutex{}
	set := func(filename string, rslt api.AnnotResult) {
		m.Lock()
		defer m.Unlock()
		_, exists := ret[filename]
		if !exists {
			ret[filename] = []api.AnnotResult{rslt}
		} else {
			// if rslt.QueuedAt.After(old.QueuedAt) {
			ret[filename] = append(ret[filename], rslt)
			// }
		}
	}
	go func() { // this is unnecessary
		defer wg.Done()
		for an := range annots {
			// fmt.Printf("Collecting %v\n", an.Path)
			config := an.GetConfig()
			set(config.Path, an)
		}
	}()
	wg.Wait()
	out <- ret
}

func GetChangedProps(a, b interface{}) ([]string, []diff.Change, error) {
	changes, err := diff.Diff(a, b)
	if err != nil {
		return nil, nil, err
	}
	ret, ret2 := []string{}, []diff.Change{}
	for _, c := range changes {
		ret = append(ret, c.Path[0])
		ret2 = append(ret2, c)
	}
	return ret, ret2, nil
}

// TODO: this is ugly and it loses metadata
func CombineResultChangesets(results []api.AnnotResult) (map[string]string, error) {
	valmap := map[string]string{}
	for i, res := range results {
		cur, err := api.ToChangeset(res)
		if err != nil {
			return nil, err
		}
		for key, val := range cur {
			_, exists := valmap[key]
			if exists {
				return nil, fmt.Errorf("cannot flatten duplicate property %v in result #%d: %#v", key, i, res)
			}
			valmap[key] = val
		}
	}
	return valmap, nil
}

// TODO: ugghhhh
func CombineResultsIsInChangeset(results []api.AnnotResult) (func([]string) bool) {
	nonsign := mapset.NewSet()
	for _, res := range results {
		cfg := res.GetConfig()
		nonsign = nonsign.Union(cfg.MetaProps.Union(cfg.IgnoredProps))
	}
	return func (props []string) bool {
		for _, prop := range props {
			if nonsign.Contains(prop) {
				return false
			}
		}
		return true
	}
}


func CombineResults(results []api.AnnotResult) []*api.RuleResults {
	ret := []*api.RuleResults{}
	for _, res := range results {
		ret = append(ret, api.ToRuleResult(res)...)
	}
	return ret
}


func GenerateChangelist(scan int, oldFiles []api.FileHistory, curFiles map[string][]api.AnnotResult) ([]api.FileHistory, error) {
	old := mapset.NewSet()
	cur := mapset.NewSet()
	oldmap := map[string]*api.FileHistory{}
	for i, r := range oldFiles {
		if !old.Add(r.Filename) {
			println("Duplicate filename found in old files, using first record: ", r.Filename)
		}
		oldmap[r.Filename] = &oldFiles[i]
	}
	for fn := range curFiles {
		cur.Add(fn)
	}

	deleted := old.Difference(cur)
	created := cur.Difference(old)
	modified := mapset.NewSet()
	// unchanged := mapset.NewSet()
	for fni := range old.Intersect(cur).Iter() {
		fn := fni.(string)
		curResults, err := CombineResultChangesets(curFiles[fn])
		if err != nil {
			return nil, err
		}
		isSignificant := CombineResultsIsInChangeset(curFiles[fn])
		// TODO: respect RuleID and Priority when overwriting values
		oldResults := map[string]string{}
		for _, rr := range oldmap[fn].RuleResults {
			oldResults[*rr.Tag] = *rr.Value
		}
		changedProps, to, err := GetChangedProps(oldResults, curResults)
		if err != nil {
			println(err)
			print(to)
			continue // don't mark errors as modified as that will flood the database with bogus modifications (TODO: allow for error type)
		}
		if len(changedProps) > 0 && isSignificant(changedProps){
			modified.Add(fn)
		} else {
			// unchanged.Add(fn)
		}
	}

	ret := []api.FileHistory{}
	allI := append(append(modified.ToSlice(), created.ToSlice()...), deleted.ToSlice()...)
	// all := mapset.NewSet().Union(created).Union(deleted).Union(modified)
	for _, fni := range allI {
		fn := fni.(string)
		item := api.FileHistory{
			Filename:    fn,
			ScanID:      scan,
			RuleResults: nil,
		}
		// don't reuse old RuleResults
		switch {
		case created.Contains(fn):
			item.Action = "C"
			fmt.Printf("Created: %v\n", fn)
			item.RuleResults = CombineResults(curFiles[fn])
		case deleted.Contains(fn):
			item.Action = "D"
			fmt.Printf("Deleted: %v\n", fn)
			item.PrevID = int(oldmap[fn].FileHistoryID) // TODO: remove PrevID altogether
		case modified.Contains(fn):
			item.Action = "M"
			fmt.Printf("Modified: %v\n", fn)
			item.PrevID = int(oldmap[fn].FileHistoryID)
			item.RuleResults = CombineResults(curFiles[fn])
		default:
			// unchanged
			continue
		}
		ret = append(ret, item)
	}
	// TODO: return diff results
	return ret, nil
}

type AppCredentials struct {
	domain      string
	username    string
	pass        string
	gqlEndpoint string
}

func (_ AppCredentials) FromEnv() AppCredentials {
	return AppCredentials{
		gqlEndpoint: os.Getenv("GAMTRAC_GRAPHQL_URI"),
		domain:      os.Getenv("GAMTRAC_DOMAIN"),
		username:    os.Getenv("GAMTRAC_USERNAME"),
		pass:        os.Getenv("GAMTRAC_PASSWORD"),
	}
}

/// returns a mapping [location]tmpdir ; don't forget to `defer scanner.UnmountShare(*tmpdir)` even on error
func mountPaths(paths []string, allowLocal bool, ac AppCredentials) (*map[string]MountedPath, func(), error) {
	mounts := map[string]MountedPath{}
	unmountAll := func() {
		for i := range mounts {
			mounts[i].Unmount()
		}
	}
	for _, p := range paths {
		path := filepath.Clean(p)
		if path != p {
			fmt.Printf("Simplified path `%v` to `%v`\n", p, path)
		}
		if _, ok := mounts[p]; ok {
			return &mounts, unmountAll, fmt.Errorf("cannot add %v: path %v already exists", p, path)
		}
		if p[:2] == `\\` {
			fmt.Printf("Mounting share `%v` using user %v\n", path, ac.username)
			tmpdir, err := scanner.MountShare(p, ac.domain, ac.username, ac.pass)
			if err != nil {
				return &mounts, unmountAll, err
			}
			fmt.Printf("Mounted share `%v` at `%v`\n", path, *tmpdir)
			mounts[p] = MountedPath{Destination: p, MountedAt: *tmpdir, Mounted: true}
		} else {
			if !allowLocal {
				return &mounts, unmountAll, fmt.Errorf("local mounts are not allowed: %v", p)
			}
			mounts[p] = MountedPath{Destination: p, MountedAt: path, Mounted: false}
		}
	}
	return &mounts, unmountAll, nil
}

func rulesGetLocal() []rules.RuleMatcher {
	ruleMatchers := []rules.RuleMatcher{}
	csv, err := rules.ReadCSVTable("testdata.csv")
	if err != nil {
		// panic(err)
		fmt.Fprintf(os.Stderr, "Cannot read from csv: %e\n", err)
	} else {
		ruleMatchers, err = rules.CSVToRules(csv, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot read from csv: %e\n", err)
		}
	}
	return ruleMatchers
}

// returns rules and corresponding rule_id
func rulesGetRemote(gg *api.GamtracGql) ([]rules.RuleMatcher, []int) {
	// fetch rules from the database
	remoteRules, err := gg.RunFetchRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read remote rules: %e\n", err)
		return []rules.RuleMatcher{}, []int{}
	}
	// place ignored rules first so that they take precedence
	sort.Slice(remoteRules, func(i, j int) bool {
		return (remoteRules[i].Ignore == true) && (remoteRules[j].Ignore == false)
	})
	ret := []rules.RuleMatcher{}
	idList := []int{}
	for _, rule := range remoteRules {
		rm, err := rules.NewMatcher(rule.Rule)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot parse remote rule %v: %e\n", rule.Rule, err)
			continue
		}
		ret = append(ret, *rm)
		idList = append(idList, rule.RuleID)
	}

	return ret, idList
}

func triggerScan(remotePaths []string, ac AppCredentials) (int, error) {
	gg := api.NewGamtracGql(ac.gqlEndpoint, 10000, os.Getenv("GAMTRAC_DEBUG_GQL") > "0")
	// import (prisma "gamtrac/prisma/generated/prisma-client")
	// import "context"
	// ctx := context.Background()
	// db := prisma.New(&prisma.Options{
	// 	Endpoint: ac.gqlEndpoint,
	// })
	// rev1, err := db.CreateScan(prisma.ScanCreateInput{}).Exec(ctx)
	// println(rev1)

	paths, unmountAll, err := mountPaths(remotePaths, os.Getenv("GAMTRAC_ALLOW_LOCAL") > "0", ac)
	defer unmountAll()
	if err != nil {
		return -1, err
	}

	rev, err := gg.RunCreateScan()
	if err != nil {
		return -1, err
	}
	fmt.Printf("Scan %v\n\n", *rev)
	oldFiles, err := gg.RunFetchFiles()

	localRules := rulesGetLocal()
	ruleIDList := []int{} // TODO: not the most elegant solution
	for range localRules {
		ruleIDList = append(ruleIDList, -1)
	}
	remoteRules, rrIds := rulesGetRemote(gg)
	ruleMatchers := append(localRules, remoteRules...)
	ruleIDList = append(ruleIDList, rrIds...)

	if len(ruleMatchers) == 0 {
		err = fmt.Errorf("failed to load at least one rule")
		return *rev, (err)
	}

	inputs := make(chan AnnotItem)
	output := make(chan api.AnnotResult)
	// errorsChan := make(chan FileError)

	wg := &sync.WaitGroup{}
	numWorkers := runtime.NumCPU()
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go processFile(inputs, output, wg)
	}

	done := make(chan map[string][]api.AnnotResult)
	go collectResults(output, done)

	for _, p := range *paths {
		filepath.Walk(p.MountedAt, func(path string, f os.FileInfo, err error) error {
			// TODO: propagate errors throught to the DB
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %e\n", err)
				return err
			}
			// path translation from destination to mounted dir
			relpath, err := filepath.Rel(p.MountedAt, path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %e\n", err)
				return err
			}
			destpath := filepath.Join(p.Destination, relpath)
			// slashes look hella weird with this but this is needed to normalize rules
			destpath = filepath.ToSlash(destpath)
			// append a slash at the end of directories
			if f.IsDir() && !strings.HasSuffix(destpath, "/") {
				destpath = destpath + "/"
			}
			mp := MountedPath{
				Destination: destpath,
				MountedAt:   path,
				Mounted:     false,
			}
			inputs <- AnnotItem{path: mp, fileInfo: f, queuedAt: time.Now(), rules: ruleMatchers, ruleIDs: ruleIDList}
			return nil
		})
	}

	close(inputs)
	wg.Wait()
	close(output)
	rslt := <-done
	changes, err := GenerateChangelist(int(*rev), oldFiles, rslt)
	if err != nil {
		return *rev, err
	}

	newFileIds, err := gg.RunInsertFileHistory(changes)
	if err != nil {
		return *rev, fmt.Errorf("cannot update files on server:\n%v", err)
	}
	if len(newFileIds) != len(changes) {
		return *rev, fmt.Errorf("invalid number of file records inserted: expected %v, got %v", len(changes), len(newFileIds))
	}
	// for _, nf := range fileIds {
	// 	fmt.Printf("%6d| %v\n\n", nf.FileHistoryID, nf.Filename)
	// }
	scanInfo, err := gg.RunFinishScan(*rev)
	if err != nil {
		return *rev, err
	}
	fmt.Printf("Finished scan #%v; inserted records: %v\n", *rev, *scanInfo.FileHistoriesAggregate.Aggregate.Count)

	return *rev, nil
}

func fetchDomainUsers(ac AppCredentials) ([]api.DomainUsers, error) {
	// rslt := map[string]api.AnnotResult{}
	li, err := scanner.NewConnectionInfo("biocad.loc", "biocad", ac.username, ac.pass, true, false)
	if err != nil {
		return nil, err
	}
	lc, err := scanner.LdapConnect(li)
	if err != nil {
		return nil, err
	}
	defer lc.Close()
	users, err := scanner.LdapSearchUsers(lc, "dc=biocad,dc=loc", "") // fmt.SPrintf("(objectSid=%s)\n", *owner))
	// user, err := scanner.LdapSearchUsers(lc,"dc=biocad,dc=loc", "(&(objectCategory=person)(objectClass=user)(SamAccountName=shtyreva))")
	// fmt.Println(user)
	if err != nil {
		return nil, err
	}
	domainUsers := make([]api.DomainUsers, len(users))
	for i, user := range users {
		grps := scanner.FilterGroups(user.MemberOf, []string{"DC=loc", "DC=biocad", "OU=biocad", "OU=Groups"})
		// used only to hoist list of groups into sql text[] type
		gs := []string{}
		for _, g := range grps {
			gs = append(gs, strings.Join(g, ","))
		}
		domainUsers[i] = api.DomainUsers{
			Sid:      user.ObjectSid,
			Username: user.SAMAccountName,
			Name:     user.CN,
			Groups:   strings.Join(gs, "\n"),
		}
		// fmt.Println(user)
	}
	return domainUsers, nil
}

func updateDomainUsers(gg *api.GamtracGql, ac AppCredentials) {
	domainUsers, err := fetchDomainUsers(ac)
	if err != nil {
		panic(err)
	}
	err = gg.RunDeleteDomainUsers()
	if err != nil {
		panic(err)
	}
	err = gg.RunInsertDomainUsers(domainUsers)
	if err != nil {
		panic(err)
	}
}

func main() {
	ac := AppCredentials{}.FromEnv()
	// fetchDomainUsers(ac)
	revDelay := os.Getenv("GAMTRAC_SCAN_DELAY")
	delay, err := strconv.Atoi(revDelay)
	if err != nil || delay < 0 {
		delay = 10
	}

	argPaths := os.Args[1:]

	for {
		rev, err := triggerScan(argPaths, ac)
		if err != nil {
			fmt.Printf("Could not finish scan %v: %v\n", rev, err)
		} else {
			fmt.Printf("Scan %v created successfully\n", rev)
		}
		time.Sleep(time.Second * time.Duration(delay))
	}
}
