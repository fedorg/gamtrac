package main

import (
	"gamtrac/api"
	"gamtrac/scanner"
	"gamtrac/rules"
	"time"
)

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
