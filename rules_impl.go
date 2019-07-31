package main

import (
	"gamtrac/api"
	"gamtrac/scanner"
	"gamtrac/rules"
	"time"
	"bytes"
	"fmt"
	"os/exec"
	"os"
	"strings"
	"path/filepath"
)

import 	"github.com/tealeg/xlsx"


// TODO: rule violations

type RuleResultGenerator interface {
	Generate(rule api.Rules, input AnnotItem) api.AnnotResult
}

type FilePropsHandler struct{ RuleResultGenerator }

func (*FilePropsHandler) Generate(rule api.Rules, input AnnotItem) api.AnnotResult {
	errors := []FileError{}
	destination := input.path.Destination
	// fmt.Println("Processing file: ", mountedAt)
	mountedAt := input.path.MountedAt
	info := input.fileInfo
	owner, err := scanner.GetFileOwnerUID(mountedAt)
	if err != nil {
		errors = append(errors, api.NewFileError(err))
	}
	var hash *HashDigest = nil
	if !info.IsDir() {
		hash, err = computeHash(mountedAt)
		if err != nil {
			errors = append(errors, api.NewFileError(err))
		}
	}
	ret := api.FilePropsResult{
		ProcessedAt: time.Now(),
		QueuedAt:    input.queuedAt,
		RuleID:      rule.RuleID,
		Path:        destination,
		MountDir:    mountedAt,
		Size:        info.Size(),
		Mode:        info.Mode(),
		ModTime:     info.ModTime(),
		IsDir:       info.IsDir(),
		OwnerUID:    owner,
		Hash:        hash,
		Errors:      errors,
	}
	return &ret
}

type PathTagsHandler struct{ RuleResultGenerator }

func (*PathTagsHandler) Generate(r api.Rules, input AnnotItem) api.AnnotResult {
	rule := r.Rule
	destination := input.path.Destination
	ruleResult := api.PathTagsResult{
		Path:   destination,
		RuleID: r.RuleID,
		Values: map[string]string{},
	}
	rm, err := rules.NewMatcher(rule)
	if err != nil {
		// TODO: return api.ErrorResult
		return &ruleResult
	}
	// match a single rule
	matches := rules.MatchAllRules(destination, []rules.RuleMatcher{*rm})
	i := rules.FindBestRuleIndex(matches)
	if i != -1 {
		m := matches[i]
		ruleResult = api.PathTagsResult{
			Path:   destination,
			RuleID: r.RuleID,
			Values: m.AsMap(),
		}
	}
	return &ruleResult

}

type MagellanWspHandler struct{ RuleResultGenerator }

func (*MagellanWspHandler) Generate(r api.Rules, input AnnotItem) api.AnnotResult {
	// rule := r.Rule

	runPolywog := func(fn string) map[string]string {
		cmd := exec.Command("./polywog", fn)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := cmd.Run()
		if err != nil {
			fmt.Println("Error: %v : Polywog failed with %s \n", fn, err)
		}
		outStr := string(stdout.Bytes())
		outs := strings.Split(outStr, "\n")
		ol := len(outs)
		writingMsg := "Writing the output to: "
		if (ol < 5 || outs[ol-2] != "Done." || outs[ol-3][:len(writingMsg)] != writingMsg) {
			return map[string]string{}
		}
		outfile := outs[ol-3][len(writingMsg):]
		defer os.Remove(outfile)
		// println(outfile)
		xf, err := xlsx.OpenFile(outfile)
		if err != nil {
			return map[string]string{}
		}
		sheets, err := xf.ToSlice()
		ret := map[string]string{}
		for _, rows := range sheets {
			if (len(rows) < 2) {
				return map[string]string{}
			}
			for _, row := range rows[1:] {
				for ncol, val := range row {
					colname := rows[0][ncol]
					ret[colname] = val
				}
			}
		}

		return ret
		// return map[string]string{
		// 	"magellan_path": outfile,
		// }
	}
	annot := map[string]string{}
	fn := input.path.MountedAt
	if (!input.fileInfo.IsDir() && (strings.ToLower(filepath.Ext(fn)) == ".wsp")) {
		annot = runPolywog(fn)
	}
	destination := input.path.Destination
	ruleResult := api.MagellanWspResult{
		Path:   destination,
		RuleID: r.RuleID,
		Values: annot,
	}
	return &ruleResult
}
