package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type ruleToken struct {
	isConst    bool
	rulePos    [2]int
	descr      string
	terminator string
}

type ruleMatcher struct {
	rule   string
	tokens []ruleToken
}

type tokenMatch struct {
	token    *ruleToken
	match    string
	matchPos [2]int
}

type ruleMatch struct {
	full    bool
	matches []tokenMatch
	lastPos int
	err     error
}

func (r *ruleMatch) AsMap() map[string]string {
	ret := make(map[string]string)
	for _, m := range r.matches {
		ret[m.token.descr] = m.match
	}
	return ret
}

func NewMatcher(rule string) (*ruleMatcher, error) {
	re := regexp.MustCompile("<[^>]+>")
	v := re.FindStringIndex(rule)
	offset := 0
	places := [][3]int{}
	for v != nil {
		start, stop := v[0]+offset, v[1]+offset
		if (start == offset) && (start != 0) {
			return nil, fmt.Errorf("Invalid rule: no spacer at location %d: %s", start, rule[:start])
		}
		if start != offset {
			places = append(places, [3]int{1, offset, start})
		}
		places = append(places, [3]int{0, start, stop})
		offset = stop
		v = re.FindStringIndex(rule[stop:])
	}
	if offset != len(rule) {
		// add the trailing const bit
		places = append(places, [3]int{1, offset, len(rule)})
		offset = len(rule)
	}
	tokens := []ruleToken{}
	for _, p := range places {
		isconst, start, stop := p[0], p[1], p[2]
		tokens = append(tokens, ruleToken{isConst: (isconst > 0), rulePos: [2]int{start, stop}, descr: rule[start:stop], terminator: ""})
	}
	for i, t := range tokens[1:] {
		if (t.isConst) && (!tokens[i].isConst) {
			tokens[i].terminator = t.descr
		}
	}
	// fmt.Println("Tokens:", tokens)
	return &ruleMatcher{rule: rule, tokens: tokens}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (tok *ruleToken) FindMatch(subject string, offset int) (*tokenMatch, error) {
	if len(subject) == 0 {
		return nil, fmt.Errorf("Cannot match empty subject")
	}
	var start, stop int
	start = offset
	if tok.isConst {
		stop = min(start+len(tok.descr), len(subject))
		if subject[start:stop] != tok.descr {
			return nil, fmt.Errorf("Failed to match on separator `%s`", tok.descr)
		}
	} else {
		stop = len(subject)
		if tok.terminator != "" {
			idx := strings.Index(subject[start:], tok.terminator)
			if idx != -1 {
				stop = start + idx
			} else {
				return nil, fmt.Errorf("Failed to find terminator `%s`", tok.terminator)
			}
		}
	}
	return &tokenMatch{token: tok, matchPos: [2]int{start, stop}, match: subject[start:stop]}, nil
}

func (r *ruleMatcher) Match(filename string) ([]tokenMatch, error) {
	offset := 0
	ret := []tokenMatch{}
	for i := range r.tokens {
		match, err := r.tokens[i].FindMatch(filename, offset)
		if err != nil {
			return ret, fmt.Errorf("Failed to match token `%s`: %s", r.tokens[i].descr, err.Error())
		}
		ret = append(ret, *match)
		offset = match.matchPos[1]
	}
	return ret, nil
}

func MatchAllRules(filename string, rules []ruleMatcher) []ruleMatch {
	getLastPos := func(tokens []tokenMatch) int {
		if len(tokens) == 0 {
			return 0
		}
		return tokens[len(tokens)-1].matchPos[1]
	}
	matches := []ruleMatch{}
	for _, matcher := range rules {
		tokens, err := matcher.Match(filename)
		lastpos := getLastPos(tokens)
		matchResult := ruleMatch{
			full:    len(filename) == lastpos,
			err:     err,
			lastPos: lastpos,
			matches: tokens,
		}
		matches = append(matches, matchResult)
	}
	return matches
}

// FindBestRuleIndex returns the index of the beset match in the list or -1 if no rules match at all
func FindBestRuleIndex(parsed []ruleMatch) int {
	ret := -1
	lastPos := 0
	for i, rm := range parsed {
		if rm.lastPos > lastPos {
			lastPos = rm.lastPos
			ret = i
		}
		if rm.full {
			return i
		}
	}
	return ret
}

func ParseFilename(filename string, rules []ruleMatcher, onlyFull bool) (*ruleMatch, error) {
	matches := MatchAllRules(filename, rules)
	i := FindBestRuleIndex(matches)
	if (i == -1) || (onlyFull && !matches[i].full) {
		return nil, fmt.Errorf("No rules match the filename `%s`", filename)
	}
	return &matches[i], nil
}

type strMap = map[string]string

func ReadCSVTable(filename string) ([]strMap, error) {
	csvFile, _ := os.Open(filename)
	reader := csv.NewReader(bufio.NewReader(csvFile))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("Invalid CSV header or no records found in `%v`", filename)
	}
	header := records[0]
	ret := []strMap{}
	for _, record := range records[1:] {
		// convert each record into a map
		r := strMap{}
		for colnum, field := range header {
			r[field] = record[colnum]
		}
		ret = append(ret, r)
	}
	return ret, nil
}

func CSVToRules(csv []strMap, convertSlashes bool) ([]ruleMatcher, error) {
	ruleStrings := []string{}
	for i, proto := range csv {
		pathElems := []string{}
		if p := proto["template_path"]; p != "" {
			pathElems = append(pathElems, p)
		}
		if p := proto["template_file"]; p != "" {
			pathElems = append(pathElems, p)
		}
		fullpath := strings.Join(pathElems, "/")
		if fullpath == "" {
			return nil, fmt.Errorf("Empty rule found at index %v", i)
		}
		if convertSlashes {
			// convert all backslashes to forward slashes
			fullpath = strings.ReplaceAll(fullpath, "\\", "/")
		}
		ruleStrings = append(ruleStrings, fullpath)
	}
	ret := []ruleMatcher{}
	for _, rs := range ruleStrings {
		rule, err := NewMatcher(rs)
		if err != nil {
			return nil, err
		}
		ret = append(ret, *rule)
	}
	return ret, nil
}
