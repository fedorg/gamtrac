package rules

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

type RuleMatcher struct {
	Rule   string
	tokens []ruleToken
}

type tokenMatch struct {
	token    *ruleToken
	match    string
	matchPos [2]int
}

type RuleMatch struct {
	Full    bool
	Matches []tokenMatch
	LastPos int
	Err     error
	Rule    *RuleMatcher
}

func (r *RuleMatch) AsMap() map[string]string {
	ret := make(map[string]string)
	for _, m := range r.Matches {
		if !m.token.isConst {
			ret[strings.Trim(m.token.descr, "<>")] = m.match
		}
	}
	return ret
}

func NewMatcher(rule string) (*RuleMatcher, error) {
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
		// if prev token is variable and this token is const, set terminator on prev token
		if (t.isConst) && (!tokens[i].isConst) {
			tokens[i].terminator = t.descr
		}
	}
	// disallow folders in last variable by default
	if len(tokens) > 0 {
		t := tokens[len(tokens)-1]
		if !t.isConst {
			t.terminator = "/"
		}
	}
	// fmt.Println("Tokens:", tokens)
	return &RuleMatcher{Rule: rule, tokens: tokens}, nil
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

func (r *RuleMatcher) Match(filename string) ([]tokenMatch, error) {
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

func MatchAllRules(filename string, rules []RuleMatcher) []RuleMatch {
	getLastPos := func(tokens []tokenMatch) int {
		if len(tokens) == 0 {
			return 0
		}
		return tokens[len(tokens)-1].matchPos[1]
	}
	matches := []RuleMatch{}
	for _, matcher := range rules {
		tokens, err := matcher.Match(filename)
		lastpos := getLastPos(tokens)
		matchResult := RuleMatch{
			Full:    len(filename) == lastpos,
			Err:     err,
			LastPos: lastpos,
			Matches: tokens,
			Rule:    &matcher,
		}
		matches = append(matches, matchResult)
	}
	return matches
}

// FindBestRuleIndex returns the index of the beset match in the list or -1 if no rules match at all
func FindBestRuleIndex(parsed []RuleMatch) int {
	ret := -1
	lastPos := 0
	for i, rm := range parsed {
		if rm.LastPos > lastPos {
			lastPos = rm.LastPos
			ret = i
		}
		if rm.Full { // should short-circuit on the first rule => rule order matters
			return i
		}
	}
	return ret
}

func ParseFilename(filename string, rules []RuleMatcher, onlyFull bool) *RuleMatch {
	matches := MatchAllRules(filename, rules)
	i := FindBestRuleIndex(matches)
	if (i == -1) || (onlyFull && !matches[i].Full) {
		return nil
	}
	return &matches[i]
}

func ReadCSVTable(filename string) ([]map[string]string, error) {
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
	ret := []map[string]string{}
	for _, record := range records[1:] {
		// convert each record into a map
		r := map[string]string{}
		for colnum, field := range header {
			r[field] = record[colnum]
		}
		ret = append(ret, r)
	}
	return ret, nil
}

func CSVToRules(csv []map[string]string, convertSlashes bool) ([]RuleMatcher, error) {
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
	ret := []RuleMatcher{}
	for _, rs := range ruleStrings {
		rule, err := NewMatcher(rs)
		if err != nil {
			return nil, err
		}
		ret = append(ret, *rule)
	}
	return ret, nil
}
