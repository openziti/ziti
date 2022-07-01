package tutorial

import (
	"bytes"
	"fmt"
	markdown "github.com/MichaelMure/go-term-markdown"
	"github.com/fatih/color"
	"github.com/openziti/foundation/v2/term"
	"github.com/pkg/errors"
	"github.com/valyala/fasttemplate"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	codeActionBlockStart    = "```action:"
	codeActionBlockEnd      = "```"
	commentActionBlockStart = "<!---action:"
	commentActionBlockEnd   = "-->"
	actionMarker            = "f572d396fae9206628714fb2ce00f72e94f2258f"
)

var debug bool

func init() {
	if val, found := os.LookupEnv("DEBUG"); found && strings.EqualFold(val, "true") {
		debug = true
	}
}

func NewRunner() *Runner {
	result := &Runner{
		actionHandlers: map[string]ActionHandler{},
		variables:      map[string]interface{}{},
		LineWidth:      75,
		LeftPad:        5,
		NewLinePause:   time.Millisecond * 20,
	}
	result.RegisterActionHandlerF("echo", func(ctx *ActionContext) error {
		fmt.Printf("echo: %v", ctx.Body)
		return nil
	})
	result.RegisterActionHandlerF("userInput", result.HandlePromptForInput)
	result.RegisterActionHandlerF("pause", result.HandlePause)
	return result
}

type ActionContext struct {
	Action  string
	Headers map[string]string
	Body    string
	Runner  *Runner
}

type ActionHandler interface {
	Execute(ctx *ActionContext) error
}

type ActionHandlerF func(ctx *ActionContext) error

func (self ActionHandlerF) Execute(ctx *ActionContext) error {
	return self(ctx)
}

type Runner struct {
	LineWidth      int
	LeftPad        int
	NewLinePause   time.Duration
	AssumeDefault  bool
	actionHandlers map[string]ActionHandler
	actions        []func() error
	variables      map[string]interface{}
}

func (self *Runner) AddVariable(name string, val interface{}) {
	self.variables[name] = val
}

func (self *Runner) ClearVariable(name string) {
	delete(self.variables, name)
}

func (self *Runner) Run(source []byte) error {
	markdownSource, err := self.transformSource(source)
	if err != nil {
		return err
	}
	renderedMarkdown := markdown.Render(string(markdownSource), self.LineWidth, self.LeftPad)
	return self.runMarkdown(renderedMarkdown)
}

func (self *Runner) runMarkdown(source []byte) error {
	fmt.Println("")
	pauseWriter := NewSlowWriter(self.NewLinePause)

	for {
		next := bytes.Index(source, []byte(actionMarker))
		if next < 0 {
			if err := self.EmitTemplatized(string(source), pauseWriter); err != nil {
				return err
			}
			fmt.Println("")
			return nil
		}
		if next == 0 {
			source = source[len(actionMarker):]
			action := self.nextAction()
			if err := action(); err != nil {
				return err
			}
		} else {
			md := source[0:next]
			if err := self.EmitTemplatized(string(md), pauseWriter); err != nil {
				return err
			}
			source = source[next:]
		}
	}
}

func (self *Runner) nextAction() func() error {
	result := self.actions[0]
	self.actions = self.actions[1:]
	return result
}

func (self *Runner) transformSource(source []byte) ([]byte, error) {
	var result []byte
	for {
		if len(source) == 0 {
			return result, nil
		}

		var behavior parseBehavior
		if CodeAction.IsStarting(source) {
			behavior = CodeAction
		} else if CommentAction.IsStarting(source) {
			behavior = CommentAction
		}

		if behavior != nil {
			source = source[len(behavior.GetStartToken()):]
			endIndex := bytes.Index(source, []byte(behavior.GetEndToken()))
			if endIndex < 0 {
				return nil, errors.Errorf("no end %v found for action block", behavior.GetEndToken())
			}
			actionSource := source[:endIndex]
			source = source[endIndex+len(behavior.GetEndToken()):]

			var actionHeader, body string

			nlIndex := bytes.IndexByte(actionSource, '\n')
			if nlIndex < 1 {
				actionHeader = string(actionSource)
			} else {
				actionHeader = strings.TrimSpace(string(actionSource[:nlIndex]))
				body = strings.TrimSpace(string(actionSource[nlIndex+1 : endIndex]))
			}
			actionDef := strings.TrimSpace(actionHeader)
			action, headers, err := self.parseAction(actionDef)
			if err != nil {
				return nil, err
			}

			handler, ok := self.actionHandlers[action]
			if !ok {
				return nil, errors.Errorf("no handler found for action: %v. Available: %+v", action, self.actionIds())
			}
			ctx := &ActionContext{
				Action:  action,
				Headers: headers,
				Body:    body,
				Runner:  self,
			}
			self.actions = append(self.actions, func() error {
				return handler.Execute(ctx)
			})

			result = append(result, []byte(actionMarker)...)
		} else {
			result = append(result, source[0])
			source = source[1:]
		}
	}
}

func (self *Runner) parseAction(s string) (string, map[string]string, error) {
	var action string
	headers := map[string]string{}

	idx := strings.IndexByte(s, ' ')
	if idx < 1 {
		return s, headers, nil
	}

	action = s[:idx]
	rest := s[idx+1:]
	for len(rest) > 0 {
		idx = strings.IndexByte(rest, '=')
		if idx < 0 {
			headers[rest] = ""
			if debug {
				fmt.Printf("%v: added %v=%v\n", action, rest, headers[rest])
			}
			rest = ""
		} else if idx == 0 {
			return "", nil, errors.Errorf("unable to parse action %v, unexpected =", action)
		} else {
			key := rest[:idx]
			rest = rest[idx+1:]
			if rest[0] == '\'' {
				rest = rest[1:]
				closingQuoteIdx := strings.IndexByte(rest, '\'')
				if closingQuoteIdx < 0 {
					return "", nil, errors.Errorf("unable to parse action %v, unclosed single-quote", action)
				}
				headers[key] = rest[:closingQuoteIdx]
				if debug {
					fmt.Printf("%v: added %v=%v\n", action, key, headers[key])
				}
				rest = rest[closingQuoteIdx+1:]
				if len(rest) > 0 {
					if rest[0] != ' ' {
						return "", nil, errors.Errorf("unable to parse action %v, expected space after closing singe-quote", action)
					} else {
						rest = rest[1:]
					}
				}
			} else {
				idx = strings.IndexByte(rest, ' ')
				if idx < 0 {
					headers[key] = rest
					if debug {
						fmt.Printf("%v: added %v=%v\n", action, key, headers[key])
					}
					rest = ""
				} else if idx == 0 {
					headers[key] = ""
					if debug {
						fmt.Printf("%v: added %v=%v\n", action, key, headers[key])
					}
					rest = rest[1:]
				} else {
					headers[key] = rest[:idx]
					if debug {
						fmt.Printf("%v: added %v=%v\n", action, key, headers[key])
					}
					rest = rest[idx+1:]
				}
			}
		}
	}

	return action, headers, nil
}

func (self *Runner) actionIds() []string {
	var result []string
	for k := range self.actionHandlers {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func (self *Runner) RegisterActionHandler(action string, handler ActionHandler) {
	self.actionHandlers[action] = handler
}

func (self *Runner) RegisterActionHandlerF(action string, handler func(ctx *ActionContext) error) {
	self.actionHandlers[action] = ActionHandlerF(handler)
}

func (self *Runner) LeftPadBuilder(b *strings.Builder) {
	for i := 0; i < self.LeftPad; i++ {
		b.WriteRune(' ')
	}
}

func (self *Runner) EmitTemplatized(s string, out io.Writer) error {
	_, err := fasttemplate.ExecuteFunc(s, "${", "}", out, func(w io.Writer, tag string) (int, error) {
		tag = strings.ReplaceAll(tag, "\n", "") // in case we hit a line-break
		tag = strings.ReplaceAll(tag, " ", "")  // in case we hit a line-break

		val, ok := self.variables[tag]
		if !ok {
			return 0, errors.Errorf("unknown template value: '%v'", tag)
		}
		buf := []byte(fmt.Sprintf("%v", val))
		return w.Write(buf)
	})
	return err
}

func (self *Runner) Template(s string) (string, error) {
	buf := &strings.Builder{}
	_, err := fasttemplate.ExecuteFunc(s, "${", "}", buf, func(w io.Writer, tag string) (int, error) {
		val, ok := self.variables[tag]
		if !ok {
			return 0, errors.Errorf("unknown template value: '%v'", tag)
		}
		buf := []byte(fmt.Sprintf("%v", val))
		return w.Write(buf)
	})
	return buf.String(), err
}

func (self *Runner) HandlePromptForInput(ctx *ActionContext) error {
	varName, ok := ctx.Headers["variable"]
	if !ok {
		return errors.New("prompt must specify which variable to set")
	}
	var val string
	var err error
	for val == "" {
		val, err = term.Prompt(ctx.Body + " ")
		if err != nil {
			return err
		}
	}
	ctx.Runner.AddVariable(varName, val)
	return nil
}

func (self *Runner) HandlePause(*ActionContext) error {
	if !self.AssumeDefault {
		c := color.New(color.FgHiWhite, color.Bold)
		_, err := term.Prompt(c.Sprintf("Press enter to continue: "))
		return err
	}
	return nil
}

type parseBehavior interface {
	IsStarting(source []byte) bool
	GetStartToken() string
	GetEndToken() string
}

type CodeActionBlock struct{}

func (self CodeActionBlock) IsStarting(source []byte) bool {
	return startsWith(source, []byte(codeActionBlockStart))
}

func (self CodeActionBlock) GetStartToken() string {
	return codeActionBlockStart
}

func (self CodeActionBlock) GetEndToken() string {
	return codeActionBlockEnd
}

type CommentActionBlock struct{}

func (self CommentActionBlock) IsStarting(source []byte) bool {
	return startsWith(source, []byte(commentActionBlockStart))
}

func (self CommentActionBlock) GetStartToken() string {
	return commentActionBlockStart
}

func (self CommentActionBlock) GetEndToken() string {
	return commentActionBlockEnd
}

func startsWith(b []byte, val []byte) bool {
	if len(b) < len(val) {
		return false
	}
	return bytes.Equal(b[:len(val)], val)
}

var CodeAction = CodeActionBlock{}
var CommentAction = CommentActionBlock{}
