package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type annotItem struct {
	path     string
	fileInfo os.FileInfo
	owner    *user.User
	rules    []ruleMatcher
}

type annotResult struct {
	path    string
	IsDir   bool
	ModTime time.Time
	Size    uint64
	Mode    uint32
	hash    hashDigest
	pattern string
	parsed  map[string]string
}

type hashDigest []byte

var test_rules = []string{
	"<дата>_<проект>_<методика>_<вид данных>_<наименование образца>_<комментарий>",
	"R:\\DAR\\LAM\\Screening group\\<Заказчик>\\1_Результаты, протоколы, отчеты\\<Измеряемый параметр>_<Метод анализа>\\<Проект>\\"}

// TODO: check if the worker pool is truly parallel by testing it with time delays

func computeHash(path string) (hashDigest, error) {
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
	digest := h.Sum(nil)
	return digest, nil
}

func processFile(inputs <-chan annotItem, output chan<- annotResult, errorsChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	for input := range inputs {
		filename := input.path
		info := input.fileInfo
		// fmt.Println("Recv files: ", filename)
		if info.IsDir() {
			continue
		}
		parsed, err := ParseFilename(filename, input.rules, true)
		if err != nil {
			errorsChan <- err
			continue
		}
		hash, err := computeHash(filename)
		if err != nil {
			// errorsChan <- err
			// continue
			hash = nil
		}
		ret := annotResult{path: filename, pattern: parsed.rule.rule, parsed: parsed.AsMap(), hash: hash}
		// fmt.Println("Send for files: ", filename)
		output <- ret
	}
}

func main() {
	username := os.Getenv("GAMTRAC_USERNAME")
	pass := os.Getenv("GAMTRAC_PASSWORD")
	// rslt := map[string]annotResult{}
	owner, err := GetFileOwnerUID(`C:\Users\fed00\Desktop\2019.02.19 DI FAVEA\03-Data-Management-and-Integrity-3-RU.pdf`)
	owner, err = GetFileOwnerUID("testdata.csv")
	owner, err = GetFileOwnerUID(`R:\DAR\ОБИ\archive\Raw Data Guava S1.3.L32-24.004 (А-0005492)\Raw Data\2018-10-20_test.fcs`)
	li, err := NewConnectionInfo("SERVER-DC3.biocad.loc", "biocad", username, pass, false, false)
	if err != nil {
		panic(err)
	}
	lc, err := LdapConnect(li)
	if err != nil {
		panic(err)
	}
	defer lc.Close()
	users, err := LdapSearchUsers(lc, "dc=biocad,dc=loc", fmt.Sprintf("(objectSid=%s)", *owner))
	if err != nil {
		panic(err)
	}
	for _, user := range users {
		fmt.Println(user)
	}

	if err != nil {
		panic(err)
	}
	fmt.Print(*owner)
	// Serve()
	csv, err := ReadCSVTable("testdata.csv")
	if err != nil {
		panic(err)
	}
	rules, err := CSVToRules(csv, true)
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
	inputs := make(chan annotItem)
	output := make(chan annotResult)
	errorsChan := make(chan error)

	wg := &sync.WaitGroup{}
	numWorkers := runtime.NumCPU()
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go processFile(inputs, output, errorsChan, wg)
	}

	printResults := func(annots <-chan annotResult, errs <-chan error, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case an := <-annots:
				// fmt.Println(hex.EncodeToString(an.hash))
				fmt.Println(base64.StdEncoding.EncodeToString(an.hash), an.path, an.pattern)
				fmt.Println(an.parsed)
				// fmt.Println("Printing results: ", an)
			case err := <-errs:
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}

	// wg.Add(1)
	go printResults(output, errorsChan, wg)
	// <-done

	for _, p := range paths {
		filepath.Walk(p, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return err
			}
			// fmt.Printf("Queued: %s\n", path)
			func() { inputs <- annotItem{path: path, fileInfo: f, rules: rules} }()
			return nil
		})
	}

	close(inputs)
	wg.Wait()
	// TODO: wait for all the result combos to finish

	// wait for everyone

}
