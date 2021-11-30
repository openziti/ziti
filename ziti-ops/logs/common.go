package logs

import (
	"bufio"
	"fmt"
	"github.com/Jeffail/gabs/v2"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ParseContext struct {
	path       string
	lineNumber int
	eof        bool
	line       string
}

func ScanLines(ctx *ParseContext, callback func(ctx *ParseContext) error) error {
	file, err := os.Open(ctx.path)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		ctx.line = line
		if err := callback(ctx); err != nil {
			return errors.Wrapf(err, "error parsing %v on line %v", ctx.path, ctx.lineNumber)
		}
		ctx.lineNumber++
	}
	ctx.eof = true
	return callback(ctx)
}

type JsonParseContext struct {
	ParseContext
	entry *gabs.Container
}

func (self *JsonParseContext) RequiredString(path ...string) (string, error) {
	v := self.entry.Search(path...)
	if v == nil || v.Data() == nil {
		return "", errors.Errorf("%v not found in log entry", strings.Join(path, "."))
	}
	s, ok := v.Data().(string)
	if !ok {
		return "", errors.Errorf("%v is not a string", strings.Join(path, "."))
	}
	return s, nil
}

func (self *JsonParseContext) ParseJsonEntry() error {
	input := self.line
	idx := strings.IndexByte(self.line, '{')
	if idx < 0 {
		fmt.Printf("WARN: non-json line: %v", self.line)
		return nil
	}
	if idx > 0 {
		input = input[idx:]
	}
	entry, err := gabs.ParseJSON([]byte(input))
	if err != nil {
		return err
	}
	self.entry = entry
	return nil
}

func ScanJsonLines(ctx *JsonParseContext, callback func(ctx *JsonParseContext) error) error {
	return ScanLines(&ctx.ParseContext, func(*ParseContext) error {
		if ctx.eof {
			return callback(ctx)
		}
		if err := ctx.ParseJsonEntry(); err != nil {
			return err
		}
		if ctx.entry != nil {
			return callback(ctx)
		}
		return nil
	})
}

type LogMatcher interface {
	Matches(ctx *JsonParseContext) (bool, error)
}

type LogFilter interface {
	LogMatcher
	Id() string
	Label() string
	Desc() string
}

type filter struct {
	LogMatcher
	id    string
	label string
	desc  string
}

func (self *filter) Id() string {
	return self.id
}

func (self *filter) Label() string {
	return self.label
}

func (self *filter) Desc() string {
	return self.desc
}

type JsonLogsParser struct {
	bucketSize                  time.Duration
	currentBucket               time.Time
	filters                     []LogFilter
	bucketMatches               map[LogFilter]int
	unmatched                   int
	maxUnmatchedLoggedPerBucket int
	ignore                      []string
}

func (self *JsonLogsParser) validate() error {
	ids := map[string]int{}
	for idx, k := range self.filters {
		if v, found := ids[k.Id()]; found {
			return errors.Errorf("duplicate filter id %v at indices %v and %v", k.Id(), idx, v)
		}
		ids[k.Id()] = idx
	}
	return nil
}

func (self *JsonLogsParser) ShowCategories(*cobra.Command, []string) {
	for _, filter := range self.filters {
		fmt.Printf("%v (%v): %v\n", filter.Id(), filter.Label(), filter.Desc())
	}
}

func (self *JsonLogsParser) examineLogEntry(ctx *JsonParseContext) error {
	if ctx.eof {
		self.dumpBucket()
		return nil
	}
	if err := self.bucket(ctx); err != nil {
		return err
	}

	for _, filter := range self.filters {
		match, err := filter.Matches(ctx)
		if err != nil {
			return err
		}

		if match {
			current := self.bucketMatches[filter]
			self.bucketMatches[filter] = current + 1
			return nil
		}
	}

	self.unmatched++
	if self.unmatched <= self.maxUnmatchedLoggedPerBucket {
		fmt.Printf("WARN: unmatched line: %v\n\n", ctx.line)
	}

	return nil
}

func (self *JsonLogsParser) bucket(ctx *JsonParseContext) error {
	s, err := ctx.RequiredString("time")
	if err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return errors.Errorf("time is in an unexpected format: %v", s)
	}
	interval := t.Truncate(self.bucketSize)
	if interval != self.currentBucket {
		if !self.currentBucket.IsZero() {
			self.dumpBucket()
		}
		self.currentBucket = interval
		self.bucketMatches = map[LogFilter]int{}
		self.unmatched = 0
	}
	return nil
}

func (self *JsonLogsParser) dumpBucket() {
	var filters []LogFilter
	for k := range self.bucketMatches {
		if !stringz.Contains(self.ignore, k.Id()) {
			filters = append(filters, k)
		}
	}
	sort.Slice(filters, func(i, j int) bool {
		return filters[i].Label() < filters[j].Label()
	})
	if len(filters) == 0 && self.unmatched == 0 {
		return
	}
	fmt.Printf("%v\n---------------------------------------------------\n", self.currentBucket.Format(time.RFC3339))
	for _, filter := range filters {
		fmt.Printf("    %v (%v): %0000v\n", filter.Id(), filter.Label(), self.bucketMatches[filter])
	}
	if self.unmatched > 0 {
		fmt.Printf("    unmatched: %0000v\n", self.unmatched)
	}
	fmt.Println()
}

func FieldStartsWith(field, substring string) LogMatcher {
	return &EntryFieldStartsWithMatcher{
		field:  field,
		prefix: substring,
	}
}

type EntryFieldStartsWithMatcher struct {
	field  string
	prefix string
}

func (self *EntryFieldStartsWithMatcher) Matches(ctx *JsonParseContext) (bool, error) {
	fieldValue, err := ctx.RequiredString(self.field)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(fieldValue, self.prefix), nil
}

func FieldContains(field, substring string) LogMatcher {
	return &EntryFieldContainsMatcher{
		field:     field,
		substring: substring,
	}
}

type EntryFieldContainsMatcher struct {
	field     string
	substring string
}

func (self *EntryFieldContainsMatcher) Matches(ctx *JsonParseContext) (bool, error) {
	fieldValue, err := ctx.RequiredString(self.field)
	if err != nil {
		return false, err
	}
	return strings.Contains(fieldValue, self.substring), nil
}

func FieldEquals(field, substring string) LogMatcher {
	return &EntryFieldContainsMatcher{
		field:     field,
		substring: substring,
	}
}

type EntryFieldEqualsMatcher struct {
	field string
	value string
}

func (self *EntryFieldEqualsMatcher) Matches(ctx *JsonParseContext) (bool, error) {
	fieldValue, err := ctx.RequiredString(self.field)
	if err != nil {
		return false, err
	}
	return fieldValue == self.value, nil
}

func AndMatchers(matchers ...LogMatcher) LogMatcher {
	return &AndMatcher{matchers: matchers}
}

type AndMatcher struct {
	matchers []LogMatcher
}

func (self *AndMatcher) Matches(ctx *JsonParseContext) (bool, error) {
	for _, matcher := range self.matchers {
		result, err := matcher.Matches(ctx)
		if !result || err != nil {
			return result, err
		}
	}
	return true, nil
}

func FieldMatches(field, expr string) LogMatcher {
	regex, err := regexp.Compile(expr)
	if err != nil {
		panic(err)
	}
	return &EntryFieldMatchesMatcher{
		field: field,
		regex: regex,
	}
}

type EntryFieldMatchesMatcher struct {
	field string
	regex *regexp.Regexp
}

func (self *EntryFieldMatchesMatcher) Matches(ctx *JsonParseContext) (bool, error) {
	fieldValue, err := ctx.RequiredString(self.field)
	if err != nil {
		return false, err
	}
	return self.regex.MatchString(fieldValue), nil
}
