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
	"strings"
	"sync"
	"time"

	"github.com/fatih/structs"
)

type HashDigest = api.HashDigest
type FileError = api.FileError
type AnnotResult = api.AnnotResult
type Files = api.Files
type ServerState = api.ServerState

type AnnotItem struct {
	path     string
	fileInfo os.FileInfo
	rules    []rules.RuleMatcher
	queuedAt time.Time
}

var test_rules = []string{
	"<дата>_<проект>_<методика>_<вид данных>_<наименование образца>_<комментарий>",
	"R:\\DAR\\LAM\\Screening group\\<Заказчик>\\1_Результаты, протоколы, отчеты\\<Измеряемый параметр>_<Метод анализа>\\<Проект>\\"}

func computeHash(path string) (*HashDigest, error) {
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
	return FileError{
		// Filename:  filename,
		Error:     err,
		CreatedAt: time.Now(),
	}
}

func processFile(inputs <-chan AnnotItem, output chan<- *AnnotResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for input := range inputs {
		filename := input.path
		info := input.fileInfo
		// fmt.Println("Processing file: ", filename)
		var rule *string
		var ruleVars map[string]string
		// TODO: run these as goroutines in parallel
		errors := make([]FileError, 0)
		parsed, err := rules.ParseFilename(strings.ReplaceAll(filename, "\\", "/"), input.rules, true)
		if err != nil {
			errors = append(errors, NewFileError(err))
		} else {
			rule = &parsed.Rule.Rule
			ruleVars = parsed.AsMap()
		}
		owner, err := scanner.GetFileOwnerUID(filename)
		if err != nil {
			errors = append(errors, NewFileError(err))
		}
		hash, err := computeHash(filename)
		if err != nil {
			errors = append(errors, NewFileError(err))
		}
		ret := api.NewAnnotResult(filename, info.Size(), info.Mode(), info.ModTime(), input.queuedAt, time.Now(), info.IsDir(), owner, hash, rule, &ruleVars, errors)
		// fmt.Println("Finished processing file: ", filename)
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
			// fmt.Printf("Collecting %v", an.Path)
			set(an.Path, an)
		}
	}()
	wg.Wait()
	out <- ret
}

func PushFileUpdates(gg *api.GamtracGql, revision int, rslts map[string]*AnnotResult) ([]api.Files, error) {
	newFiles := make([]Files, len(rslts))
	i := 0
	for filename, r := range rslts {
		data := structs.Map(r)
		newFiles[i] = Files{
			Filename:   filename,
			RevisionID: revision,
			Data:       data,
		}
		i++
	}

	insertedFiles, err := gg.RunUpsertFiles(newFiles)
	if err != nil {
		return nil, err
	}
	// patch returned file ids back into new files
	if len(insertedFiles) != len(newFiles) {
		return nil, fmt.Errorf("invalid number of file records inserted: expected %v, got %v", len(newFiles), len(insertedFiles))
	}
	for i, dbfile := range insertedFiles {
		newFiles[i].FileID = dbfile.FileID
	}
	// delete old files that are not in this revision
	// TODO: this should not delete files under untracked folders
	_, err = gg.RunDeleteFiles(revision)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
	}
	// TODO: this function has shit error api
	return insertedFiles, nil
}

func ListToMap(list []Files) map[string]*Files {
	ret := map[string]*Files{}
	for i, r := range list {
		ret[r.Filename] = &list[i]
	}
	return ret
}

func main() {
	gg := api.NewGamtracGql("https://fedor-hasura-test.herokuapp.com/v1alpha1/graphql", 5000, false)

	srvState := api.NewServerState()
	go api.Serve(srvState)

	username := os.Getenv("GAMTRAC_USERNAME")
	pass := os.Getenv("GAMTRAC_PASSWORD")
	// // rslt := map[string]AnnotResult{}
	/*
		li, err := scanner.NewConnectionInfo("biocad.loc", "biocad", username, pass, true, false)
		if err != nil {
			panic(err)
		}
		lc, err := scanner.LdapConnect(li)
		if err != nil {
			panic(err)
		}
		defer lc.Close()
		users, err := scanner.LdapSearchUsers(lc, "dc=biocad,dc=loc", "") // fmt.Sprintf("(objectSid=%s)", *owner))
		if err != nil {
			panic(err)
		}
		domainUsers := make([]api.DomainUsers, len(users))
		for i, user := range users {
			grps := scanner.FilterGroups(user.MemberOf, []string{"DC=loc", "DC=biocad", "OU=biocad", "OU=Groups"})
			// used only to hoist list of groups into sql text[] type
			domainUsers[i] = api.DomainUsers{
				Sid:      user.ObjectSid,
				Username: user.SAMAccountName,
				Name:     user.CN,
				Groups:   grps,
			}
			// fmt.Println(user)
		}
		err = gg.RunDeleteDomainUsers()
		if err != nil {
			panic(err)
		}
		err = gg.RunInsetDomainUsers(domainUsers)
		if err != nil {
			panic(err)
		}
	*/

	args := os.Args[1:]
	var paths []string
	if len(args) == 0 {
		paths = append(paths, ".")
	} else {
		paths = append(paths, args...)
	}
	for i, p := range paths {
		if p[:2] == `\\` {
			fmt.Printf("Mounting share `%v` using user %v\\%v", p, "biocad", username)
			tmpdir, err := scanner.MountShare(p, "biocad", username, pass)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				continue
			}
			defer scanner.UnmountShare(*tmpdir)
			fmt.Printf("Mounted share `%v` at `%v`", p, *tmpdir)
			paths[i] = *tmpdir // override p
		}
	}

	for {
		rev, err := gg.RunCreateRevision()
		if err != nil {
			panic(err)
		}
		epoch := *rev
		fmt.Printf("Epoch %v\n", epoch)
		csv, err := rules.ReadCSVTable("testdata.csv")
		if err != nil {
			panic(err)
		}
		rules, err := rules.CSVToRules(csv, true)
		if err != nil {
			panic(err)
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

		for _, p := range paths {
			filepath.Walk(p, func(path string, f os.FileInfo, err error) error {
				if err != nil {
					fmt.Printf("Error: %s\n", err.Error())
					// panic(err)
					return err
				}
				// fmt.Printf("Queued: %s\n", path)
				func() { inputs <- AnnotItem{path: path, fileInfo: f, queuedAt: time.Now(), rules: rules} }()
				return nil
			})
		}

		close(inputs)
		wg.Wait()
		close(output)
		rslt := <-done
		newFiles, err := PushFileUpdates(gg, int(epoch), rslt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot update files on server:\n%v\n", err)
		}
		for _, nf := range newFiles {
			fmt.Printf("%6d| %v\n", nf.FileID, nf.Filename)
		}
		srvState.Update(rslt)
		fmt.Fprintf(os.Stderr, "Finished epoch: %v\n", epoch)
		time.Sleep(time.Second * 10)

		// return
	}

}
