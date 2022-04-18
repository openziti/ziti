package zdecli

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/storage/boltz"
	"github.com/rodaine/table"
	"regexp"
	"strconv"
	"strings"
	"ziti-db-explorer/zdelib"
)

func PrintUsage() {
	println("")
	println("'ziti-db-explorer' is an interactive shell for exploring Ziti Controller database files")
	println("")
	println("")
	println("Usage: ")
	println("\tziti-db-explorer [help|version|<ctrl.db>]")
	println("")
}

func PrintVersion() {
	println("")
	fmt.Printf("Version: %s\n", zdelib.Version)
	fmt.Printf("BuildDate: %s\n", zdelib.BuildDate)
	fmt.Printf("Commit: %s\n", zdelib.Commit)
	fmt.Printf("Branch: %s\n", zdelib.Branch)
	println("")
}

func PrintHelp(state *zdelib.State, registry *CommandRegistry, _ string) error {
	tbl := table.New("command", "description")
	for _, cmdText := range registry.CommandTexts {
		action := registry.CommandTextToAction[cmdText]

		if action.IsSuggested {
			tbl.AddRow(action.Text, action.Description)
		}

	}

	tbl.Print()
	return nil
}

// PrintValue will attempt to print the value from a given `key` in the current bucket location determined by
// `state`.
func PrintValue(state *zdelib.State, _ *CommandRegistry, key string) error {
	key = strings.TrimSpace(key)

	println(state.GetValue(key))

	return nil
}

// ClearConsole will output ASCII control characters that clear the console on modern terminals.
func ClearConsole(_ *zdelib.State, _ *CommandRegistry, _ string) error {
	fmt.Printf("\033[H\033[2J")
	return nil
}

// CdBucket is an ActionHandler to `cd <bucket`. Will update the provided `state`'s location.
func CdBucket(state *zdelib.State, registry *CommandRegistry, bucketName string) error {
	bucketName = strings.TrimSpace(bucketName)

	if bucketName == ".." {
		return NavBackOne(state, registry, bucketName)
	}
	return state.Enter(bucketName)
}

// PrintCurrentCount is an ActionHandler that will print the key count for the provided `state`'s location.
func PrintCurrentCount(state *zdelib.State, _ *CommandRegistry, _ string) error {
	count := state.CurrentBucketKeyCount()

	println("")
	fmt.Printf("Count: %d\n", count)
	println("")

	return nil
}

// PrintDbStats is an ActionHandler that will print the bbolt database stats the current `state` has open.
func PrintDbStats(state *zdelib.State, _ *CommandRegistry, _ string) error {
	stats := state.DbStats()

	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	tbl := table.New("Property", "Value", "Description")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)

	tbl.AddRow("FreePageN", stats.FreePageN, "total number of free pages on the freelist")
	tbl.AddRow("PendingPageN", stats.PendingPageN, "total number of pending pages on the freelist")
	tbl.AddRow("FreeAlloc", stats.FreeAlloc, "total bytes allocated in free pages")
	tbl.AddRow("FreelistInuse", stats.FreelistInuse, "total bytes used by the freelist")
	tbl.AddRow("TxN", stats.TxN, "total number of started read transactions")
	tbl.AddRow("OpenTxN", stats.OpenTxN, "number of currently open read transactions")

	// Page statistics.
	tbl.AddRow("TxStats.PageCount", stats.TxStats.PageCount, "number of page allocations")
	tbl.AddRow("TxStats.PageAlloc", stats.TxStats.PageAlloc, "total bytes allocated")

	// Cursor statistics.
	tbl.AddRow("TxStats.CursorCount", stats.TxStats.CursorCount, "number of cursors created")

	// Node statistics
	tbl.AddRow("TxStats.NodeCount", stats.TxStats.NodeCount, "number of node allocations")
	tbl.AddRow("TxStats.NodeDeref", stats.TxStats.NodeDeref, "number of node dereferences")

	// Rebalance statistics.
	tbl.AddRow("TxStats.Rebalance", stats.TxStats.Rebalance, "number of node rebalances")
	tbl.AddRow("TxStats.RebalanceTime", stats.TxStats.RebalanceTime, "total time spent rebalancing")

	// Split/Spill statistics.
	tbl.AddRow("TxStats.Split", stats.TxStats.Split, "number of nodes split")
	tbl.AddRow("TxStats.Spill", stats.TxStats.Spill, "number of nodes spilled")
	tbl.AddRow("TxStats.SpillTime", stats.TxStats.SpillTime, "total time spent spilling")

	// Write statistics.
	tbl.AddRow("TxStats.Write", stats.TxStats.Write, "number of writes performed")
	tbl.AddRow("TxStats.WriteTime", stats.TxStats.WriteTime, "total time spent writing to disk")

	println("")
	tbl.Print()
	println("")

	return nil
}

// PrintBucketStats is an ActionHandler that will print the provided `state`'s current bucket location's stats.
func PrintBucketStats(state *zdelib.State, _ *CommandRegistry, _ string) error {
	stats := state.BucketStats()

	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	tbl := table.New("Property", "Value", "Description")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)

	// Page count statistics.
	tbl.AddRow("BranchPageN", stats.BranchPageN, " number of logical branch pages")
	tbl.AddRow("BranchOverflowN", stats.BranchOverflowN, " number of physical branch overflow pages")
	tbl.AddRow("LeafPageN", stats.LeafPageN, " number of logical leaf pages")
	tbl.AddRow("LeafOverflowN", stats.LeafOverflowN, " number of physical leaf overflow pages")

	// Tree statistics.
	tbl.AddRow("KeyN", stats.KeyN, " number of keys/value pairs")
	tbl.AddRow("Depth", stats.Depth, " number of levels in B+tree")

	// Page size utilization.
	tbl.AddRow("BranchAlloc", stats.BranchAlloc, " bytes allocated for physical branch pages")
	tbl.AddRow("BranchInuse", stats.BranchInuse, " bytes actually used for branch data")
	tbl.AddRow("LeafAlloc", stats.LeafAlloc, " bytes allocated for physical leaf pages")
	tbl.AddRow("LeafInuse", stats.LeafInuse, " bytes actually used for leaf data")

	// Bucket statistics
	tbl.AddRow("BucketN", stats.BucketN, " total number of buckets including the top bucket")
	tbl.AddRow("InlineBucketN", stats.InlineBucketN, " total number on inlined buckets")
	tbl.AddRow("InlineBucketInuse", stats.InlineBucketInuse, " bytes used for inlined buckets (also accounted for in LeafInuse)")

	println("")
	tbl.Print()
	println("")

	return nil
}

// PrintPath is an ActionHandler that will print the provided `state`'s bucket location.
func PrintPath(state *zdelib.State, _ *CommandRegistry, _ string) error {
	if len(state.Path) == 0 {
		println("root")
	} else {
		println(strings.Join(state.Path, "."))
	}

	return nil
}

// NavToRoot is an ActionHandler that will navigate the provided `state` to the root bucket.
func NavToRoot(state *zdelib.State, _ *CommandRegistry, _ string) error {
	state.Path = []string{}
	return nil
}

// PathPrompt will return a string suitable to be an interactive CLI's prompt prefix.
func PathPrompt(state *zdelib.State) string {
	promptString := ""

	size := len(state.Path)
	switch {
	case size == 0:
		promptString = "root"
		break
	case size <= 4:
		promptString = strings.Join(state.Path, ".")
		break
	default:
		promptString = state.Path[0] + "..." + strings.Join(state.Path[len(state.Path)-3:], ".")
	}
	return promptString + ">"
}

// NavBackOne is an ActionHandler that will navigate the provided `state` one bucket level back if possible.
func NavBackOne(state *zdelib.State, _ *CommandRegistry, _ string) error {
	return state.Back()
}

// ListCurrentBucketWithLimits will print a table of the provided `state`'s location's keys
// and values. If limit is zero or negative all values will be listed. A positive limit will only show that number
// of keys and values. Skip must be 0 or greater and will skip that number of keys.
func ListCurrentBucketWithLimits(state *zdelib.State, skip int64, limit int64) error {
	if limit <= 0 {
		limit = -1
	}

	if skip < 0 {
		skip = 0
	}
	entries := state.ListEntries()

	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	outTable := table.New("Key", "Type", "Value")
	outTable.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)

	bucketStr := "..."
	nilStr := "<nil>"

	numSkipped := int64(0)
	numOutput := int64(0)

	for _, entry := range entries {
		if numSkipped < skip {
			numSkipped++
			continue
		}

		if limit != -1 && numOutput > limit {
			break
		}

		numOutput++

		if entry.Type == 0 || entry.Type == boltz.TypeNil {
			entry.TypeString = "Bucket"

			entry.ValueString = &bucketStr
		}

		if entry.ValueString == nil {
			entry.ValueString = &nilStr
		}

		valLen := 50

		valString := *entry.ValueString

		valString = strings.Replace(valString, "\n", "\\n", -1)
		valString = strings.Replace(valString, "\t", "\\t", -1)

		if len(valString) < valLen {
			valLen = len(valString)
		}

		outTable.AddRow(entry.Name, entry.TypeString, valString[0:valLen])
	}

	limitStr := "no limit"
	if limit != -1 {
		limitStr = strconv.FormatInt(limit, 10)
	}

	println("")
	outTable.Print()

	if len(entries) == 0 {
		println("...dust")
	}
	println("")
	fmt.Printf("skipped: %d, limit: %s", skip, limitStr)
	println("")

	return nil
}

// ListCurrentBucketAll is an ActionHandler that will print a table of the provided `state`'s location's keys
// and values. It will print all keys from first to last.
func ListCurrentBucketAll(state *zdelib.State, _ *CommandRegistry, _ string) error {
	return ListCurrentBucketWithLimits(state, 0, -1)
}

// ListCurrentBucket is an ActionHandler that will parse `args` in order to call ListCurrentBucketWithLimits.
func ListCurrentBucket(state *zdelib.State, _ *CommandRegistry, args string) error {
	noSpaces := regexp.MustCompile(`\s\s+`)
	args = noSpaces.ReplaceAllString(args, " ")
	splits := strings.Split(args, " ")
	skip := int64(0)
	limit := int64(100)

	for i, arg := range splits {
		if arg == "--skip" {
			if i+1 < len(splits) {
				if parsed, err := strconv.ParseInt(splits[i+1], 10, 64); err == nil {
					skip = parsed
				}
			}
		}

		if arg == "--limit" {
			if i+1 < len(splits) {
				if parsed, err := strconv.ParseInt(splits[i+1], 10, 64); err == nil {
					limit = parsed
				}
			}
		}
	}

	return ListCurrentBucketWithLimits(state, skip, limit)
}
