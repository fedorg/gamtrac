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
	"strconv"
	"sync"
	"time"
	"strings"
	"encoding/json"
	"github.com/r3labs/diff"

	"github.com/fatih/structs"

)

const USE_HASH = false

type HashDigest = api.HashDigest
type FileError = api.FileError
type AnnotResult = api.AnnotResult


type MountedPath struct {
	Destination string
	MountedAt    string
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
	queuedAt time.Time
}

var test_rules = []string{
	"<дата>_<проект>_<методика>_<вид данных>_<наименование образца>_<комментарий>",
	"R:\\DAR\\LAM\\Screening group\\<Заказчик>\\1_Результаты, протоколы, отчеты\\<Измеряемый параметр>_<Метод анализа>\\<Проект>\\"}

func computeHash(path string) (*HashDigest, error) {
	if !USE_HASH {
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

func processFile(inputs <-chan AnnotItem, output chan<- *AnnotResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for input := range inputs {
		destination := input.path.Destination
		mountedAt := input.path.MountedAt
		info := input.fileInfo
		// fmt.Println("Processing file: ", mountedAt)
		var rule *string
		var ruleVars map[string]string
		errors := []FileError{}
		parsed := rules.ParseFilename(destination, input.rules, true)
		if parsed != nil {
			rule = &parsed.Rule.Rule
			ruleVars = parsed.AsMap()
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
		ret := api.NewAnnotResult(destination, mountedAt, info.Size(), info.Mode(), info.ModTime(), input.queuedAt, time.Now(), info.IsDir(), owner, hash, rule, &ruleVars, errors)
		// fmt.Println("Finished processing file: ", mountedAt)
		output <- &ret
	}
}

func collectResults(annots <-chan *AnnotResult, out chan<- map[string]*AnnotResult) {
	// TODO: make this less ugly
	ret := map[string]*AnnotResult{}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	m := &sync.Mutex{}
	set := func(filename string, rslt *AnnotResult) {
		m.Lock()
		defer m.Unlock()
		old, exists := ret[filename]
		if !exists {
			ret[filename] = rslt
		} else {
			if old.ProcessedAt.Before(rslt.ProcessedAt) {
				ret[filename] = rslt
			}
		}
	}
	go func() {
		defer wg.Done()
		for an := range annots {
			// fmt.Printf("Collecting %v\n", an.Path)
			set(an.Path, an)
		}
	}()
	wg.Wait()
	out <- ret
}

func PushFileUpdates(gg *api.GamtracGql, scan int, oldFiles []api.FileHistory, rslts map[string]*AnnotResult) ([]api.FileHistory, error) {
	newFiles := make([]api.FileHistory, len(rslts))
	i := 0
	for filename, r := range rslts {
		var data map[string]interface{}
		data = structs.Map(r)
		delete(data, "Path")
		delete(data, "MountDir")
		if r.Hash != nil {
			data["Hash"] = r.Hash.String()
		} else {
			data["Hash"] = nil
		}
		newFiles[i] = api.FileHistory{
			Filename:   filename,
			ScanID:     scan,
			RuleResults: nil,//[]*api.RuleResults {{ Data: data }},
			Action: "C",
			PrevID: 0,
		}
		i++
	}

	changes, err := CompareFileLists(oldFiles, newFiles)
	if err != nil {
		return nil, err
	}
	ctext, err := json.MarshalIndent(changes.CreatedInNew, "", " ")
	dtext, err := json.MarshalIndent(changes.RemovedFromOld, "", " ")
	mtext, err := json.MarshalIndent(changes.ModifiedInNew, "", " ")
	fmt.Printf("Created:\n%v\n", string(ctext))
	fmt.Printf("Removed:\n%v\n", string(dtext))
	fmt.Printf("Changed:\n%v\n", string(mtext))

	insertedFiles, err := gg.RunInsertFileHistory(newFiles)
	if err != nil {
		return nil, err
	}
	// patch returned file ids back into new files
	if len(insertedFiles) != len(newFiles) {
		return nil, fmt.Errorf("invalid number of file records inserted: expected %v, got %v", len(newFiles), len(insertedFiles))
	}
	for i, fileid := range insertedFiles {
		newFiles[i].FileHistoryID = fileid
	}
	// TODO: this function has shit error api
	return newFiles, nil
}


type FilePropDiff struct {
	NewPropValues map[string]string
}

type FileDiffResults struct {
	CreatedInNew   map[string]*api.FileHistory
	RemovedFromOld map[string]*api.FileHistory
	ModifiedInNew  map[string]FilePropDiff
}

func ListToMap(list []api.FileHistory) map[string]*api.FileHistory {
	ret := map[string]*api.FileHistory{}
	for i, r := range list {
		ret[r.Filename] = &list[i]
	}
	return ret
}

func CompareFileLists(oldFiles []api.FileHistory, newFiles []api.FileHistory) (*FileDiffResults, error) {
	old, new := ListToMap(oldFiles), ListToMap(newFiles)
	CreatedInNew, RemovedFromOld := map[string]*api.FileHistory{}, map[string]*api.FileHistory{}
	// go over old and detect deletes
	for filename, f := range old {
		if _, ok := new[filename]; !ok {
			RemovedFromOld[filename] = f
		}
	}
	// go over new and detect creates
	for filename, f := range new {
		if _, ok := old[filename]; !ok {
			CreatedInNew[filename] = f
		}
	}

	

	// fillStruct := func(data map[string]interface{}, recv interface{}) error {
	// 	bytes, err := json.Marshal(data)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	err = json.Unmarshal(bytes, recv)
	// 	return err
	// }

	// detect changes:
	//	over new:
	//		if in new: skip (leaves intersection)
	//		unmarshal data
	//		compare data
	//		if compare non identical, detect modified
	modified := map[string]FilePropDiff{}
	for filename := range new {
		of, inOld := old[filename]
		nf, inNew := new[filename]
		if !(inNew && inOld) {
			continue
		}
		changes, err := diff.Diff(of, nf)
		if err != nil {
			return nil, err
		}
		newProps := map[string]string{}
		for _, c := range changes {
			key := c.Path[0]
			bytes, err := json.Marshal(c.To)
			if err != nil {
				return nil, err
			}
			val := string(bytes)
			newProps[key] = val
		}
		if len(newProps) > 0 {
			modified[filename] = FilePropDiff{NewPropValues: newProps}
		}
		// outputs: deletes, creates, modified
	}
	return &FileDiffResults{
		CreatedInNew:   CreatedInNew,
		RemovedFromOld: RemovedFromOld,
		ModifiedInNew:  modified,
	}, nil
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
		for _, p := range mounts {
			p.Unmount()
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

func rulesGetRemote(gg *api.GamtracGql) []rules.RuleMatcher {
	// fetch rules from the database
	ret := []rules.RuleMatcher{}
	remoteRules, err := gg.RunFetchRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read remote rules: %e\n", err)
	} else {
		matchers := []rules.RuleMatcher{}
		ignoredMatchers := []rules.RuleMatcher{}
		for _, rule := range remoteRules {
			rm, err := rules.NewMatcher(rule.Rule)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot read remote rules: %e\n", err)
				return []rules.RuleMatcher{}
			}
			if rule.Ignore {ignoredMatchers = append(ignoredMatchers, *rm)} else {matchers = append(matchers, *rm)}
		}
		// place ignored rules first so that they take precedence
		ret = append(ret, ignoredMatchers...)
		ret = append(ret, matchers...)
	}
	return ret
}

func triggerScan(remotePaths []string, ac AppCredentials) (int, error) {
	gg := api.NewGamtracGql(ac.gqlEndpoint, 10000, true)
	// import (prisma "gamtrac/prisma/generated/prisma-client")
	// import "context"
	// ctx := context.Background()
	// db := prisma.New(&prisma.Options{
	// 	Endpoint: ac.gqlEndpoint,
	// })
	// rev1, err := db.CreateScan(prisma.ScanCreateInput{}).Exec(ctx)
	// println(rev1)

	paths, unmountAll, err := mountPaths(remotePaths, false, ac)
	defer unmountAll()
	if err != nil {
		return -1, err
	}

	rev, err := gg.RunCreateScan()
	if err != nil {
		return -1, err
	}
	oldFiles, err := gg.RunFetchFiles()
	
	epoch := *rev
	fmt.Printf("Epoch %v\n\n", epoch)

	localRules := rulesGetLocal()
	remoteRules := rulesGetRemote(gg)
	ruleMatchers := append(localRules, remoteRules...)
	if len(ruleMatchers) == 0 {
		err = fmt.Errorf("failed to load at least one rule")
		return *rev, (err)
	}

	inputs := make(chan AnnotItem)
	output := make(chan *AnnotResult)
	// errorsChan := make(chan FileError)

	wg := &sync.WaitGroup{}
	numWorkers := runtime.NumCPU()
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go processFile(inputs, output, wg)
	}

	done := make(chan map[string]*AnnotResult)
	go collectResults(output, done)

	for _, p := range *paths {
		filepath.Walk(p.MountedAt, func(path string, f os.FileInfo, err error) error {
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
			// append a slash at the end of directories
			if (f.IsDir() && !strings.HasSuffix(relpath, "/")) {
				relpath = relpath + "/"
			}
			destpath := filepath.Join(p.Destination, relpath)
			// slashes look hella weird with this but this is needed to normalize rules
			destpath = filepath.ToSlash(destpath)
			mp := MountedPath{
				Destination: destpath,
				MountedAt: path,
				Mounted: false,
			}
			func() { inputs <- AnnotItem{path: mp, fileInfo: f, queuedAt: time.Now(), rules: ruleMatchers} }()
			return nil
		})
	}

	close(inputs)
	wg.Wait()
	close(output)
	rslt := <-done
	newFiles, err := PushFileUpdates(gg, int(epoch), oldFiles, rslt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot update files on server:\n%v\n", err)
	}
	for _, nf := range newFiles {
		fmt.Printf("%6d| %v\n\n", nf.FileHistoryID, nf.Filename)
	}
	fmt.Fprintf(os.Stderr, "Finished epoch: %v\n", epoch)

	return *rev, nil
}


func fetchDomainUsers(ac AppCredentials) ([]api.DomainUsers, error) {
	// rslt := map[string]AnnotResult{}
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
			fmt.Printf("Could not finish scan %v\n", rev)
		} else {
			fmt.Printf("Scan %v created successfully\n", rev)
		}
		time.Sleep(time.Second * time.Duration(delay))
	}
}
