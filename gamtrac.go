package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
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
		fmt.Println("Processing file: ", filename)
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
		fmt.Println("Finished processing file: ", filename)
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
			fmt.Printf("Collecting %v", an.Path)
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
		data, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		jsonStr := string(data)
		newFiles[i] = Files{
			Filename: filename,
			Revision: revision,
			Data:     jsonStr,
		}
		i++
	}
	insertedFiles, err := gg.RunInsertFiles(newFiles)
	if err != nil {
		return nil, err
	}
	return insertedFiles, nil
}

func main() {
	srvState := api.NewServerState()
	go api.Serve(srvState)

	// username := os.Getenv("GAMTRAC_USERNAME")
	// pass := os.Getenv("GAMTRAC_PASSWORD")
	// // rslt := map[string]AnnotResult{}
	// owner, err := GetFileOwnerUID(`C:\Users\fed00\Desktop\2019.02.19 DI FAVEA\03-Data-Management-and-Integrity-3-RU.pdf`)
	// owner, err = GetFileOwnerUID("testdata.csv")
	// owner, err = GetFileOwnerUID(`R:\DAR\ОБИ\archive\Raw Data Guava S1.3.L32-24.004 (А-0005492)\Raw Data\2018-10-20_test.fcs`)
	// li, err := NewConnectionInfo("SERVER-DC3.biocad.loc", "biocad", username, pass, false, false)
	// if err != nil {
	// 	panic(err)
	// }
	// lc, err := LdapConnect(li)
	// if err != nil {
	// 	panic(err)
	// }
	// defer lc.Close()
	// users, err := LdapSearchUsers(lc, "dc=biocad,dc=loc", fmt.Sprintf("(objectSid=%s)", *owner))
	// if err != nil {
	// 	panic(err)
	// }
	// for _, user := range users {
	// 	fmt.Println(user)
	// }

	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Print(*owner)

	var epoch int64 = 0 // TODO create revision records on the server
	gg := api.NewGamtracGql("https://fedor-hasura-test.herokuapp.com/v1alpha1/graphql", 5000, false)

	for {
		epoch++
		fmt.Printf("Epoch %v\n", epoch)
		csv, err := rules.ReadCSVTable("testdata.csv")
		if err != nil {
			panic(err)
		}
		rules, err := rules.CSVToRules(csv, true)
		if err != nil {
			panic(err)
		}

		args := os.Args[1:]
		var paths []string
		if len(args) == 0 {
			paths = append(paths, ".")
		} else {
			paths = append(paths, args...)
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
				fmt.Printf("Queued: %s\n", path)
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
		time.Sleep(time.Second * 1)
	}
	// TODO: wait for all the result combos to finish

	// wait for everyone

}
