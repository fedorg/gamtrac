package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type annotItem struct {
	path     string
	fileInfo os.FileInfo
}

type annotResult struct {
	path    string
	hash    hashDigest
	pattern string
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
		fmt.Println("Recv files: ", filename)
		if info.IsDir() {
			continue
		}

		fields := strings.FieldsFunc(test_rules[0], func(r rune) bool { return (r == '<') || (r == '>') })
		annots := []string{path.Dir(filename), path.Base(filename), path.Ext(filename)}
		annots = append(annots, fields...)
		hash, err := computeHash(filename)
		if err != nil {
			errorsChan <- err
			continue
		}
		ret := annotResult{path: filename, pattern: strings.Join(fields, ","), hash: hash}
		fmt.Println("Send for files: ", filename)
		output <- ret
		// for _, tok := range strings.Split(filename, "<") {
		// 	tok_start := strings.Index(tok, ">")
		// 	if tok_start == -1 {
		// 		errorsChan <- errors.New(fmt.Sprintf("Invalid rule spec %s", filename))
		// 	}
		// }

		// hash, err := computeHash(filename)
		// if err != nil {
		// 	errorsChan <- err
		// }

		// scanner := bufio.NewScanner(file)
		// scanner.Split(bufio.ScanWords)
		// for scanner.Scan() {
		// 	output <- scanner.Text()
		// }
		// if scanner.Err() != nil {
		// 	errorsChan <- scanner.Err()
		// }

	}
}

func main() {
	// Serve()
	crazyrule := "<disk>:\\<folder>\\<дргуая папка>\\totally_not<what_is_it>>.<ext>:file"
	parsedRule, err := NewMatcher(crazyrule) //test_rules[0])
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(*parsedRule)

	// TODO: convert windows slashes to normal slashes and match both
	matches, err := parsedRule.Match("C:\\Windows\\System32\\totally_not_virus<3>.data:file")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(matches)

	args := os.Args[1:]
	var paths []string
	if len(args) == 0 {
		paths = append(paths, ".")
	} else {
		paths = append(paths, args...)
	}
	inputFiles := make(chan annotItem)
	output := make(chan annotResult)
	errorsChan := make(chan error)

	wg := &sync.WaitGroup{}
	numWorkers := runtime.NumCPU()
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go processFile(inputFiles, output, errorsChan, wg)
	}

	printResults := func(annots <-chan annotResult, errs <-chan error, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case an := <-annots:
				fmt.Println(hex.EncodeToString(an.hash))
				fmt.Println(base64.StdEncoding.EncodeToString(an.hash))
				fmt.Println("Printing results: ", an)
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
			fmt.Printf("Queued: %s\n", path)
			func() { inputFiles <- annotItem{path: path, fileInfo: f} }()
			return nil
		})
	}

	close(inputFiles)
	wg.Wait()
	// TODO: wait for all the result combos to finish

	// wait for everyone

}
