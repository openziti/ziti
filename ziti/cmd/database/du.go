/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package database

import (
	"fmt"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/outputz"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

type DiskUsageAction struct {
	humanReadable bool
	outputDepth   uint32
}

func NewDiskUsageAction() *cobra.Command {
	action := &DiskUsageAction{}

	cmd := &cobra.Command{
		Use:   "du <db-file>",
		Short: "Analyzes a bbolt database storage usage",
		Args:  cobra.ExactArgs(1),
		RunE:  action.Run,
	}

	cmd.Flags().BoolVarP(&action.humanReadable, "human-readable", "H", false, "human readable sizes")
	cmd.Flags().Uint32VarP(&action.outputDepth, "max-depth", "d", 0, "how many levels deep to output")

	return cmd
}

// Run implements this command
func (self *DiskUsageAction) Run(cmd *cobra.Command, args []string) error {
	srcOptions := *bbolt.DefaultOptions
	srcOptions.ReadOnly = true

	srcDb, err := bbolt.Open(args[0], 0400, &srcOptions)
	if err != nil {
		return err
	}

	root := &sizeNode{
		path: "/",
	}

	v := &sizeVisitor{
		m: map[string]*sizeNode{
			"/": root,
		},
	}

	err = srcDb.View(func(tx *bbolt.Tx) error {
		boltz.Traverse(tx, "", v)
		return nil
	})

	if err != nil {
		return err
	}

	root.calcSize()
	root.dump(self.humanReadable, self.outputDepth, 1)

	return nil
}

type sizeNode struct {
	path     string
	size     uint64
	children []*sizeNode
}

func (self *sizeNode) calcSize() {
	for _, child := range self.children {
		child.calcSize()
		self.size += child.size
	}
}

func (self *sizeNode) dump(humanReadable bool, maxDepth uint32, currentDepth uint32) {
	for _, child := range self.children {
		child.dump(humanReadable, maxDepth, currentDepth+1)
	}

	if maxDepth == 0 || currentDepth <= maxDepth {
		if humanReadable {
			fmt.Printf("%v: %v\n", self.path, outputz.FormatBytes(self.size))
		} else {
			fmt.Printf("%v: %v\n", self.path, self.size)
		}
	}
}

type sizeVisitor struct {
	m map[string]*sizeNode
}

func (self *sizeVisitor) VisitBucket(path string, key []byte, _ *bbolt.Bucket) bool {
	parent := self.getNode(path)
	selfNode := self.getNode(path + "/" + string(key))
	parent.children = append(parent.children, selfNode)
	selfNode.size += uint64(len(key))
	return true
}

func (self *sizeVisitor) VisitKeyValue(path string, key, value []byte) bool {
	parent := self.getNode(path)
	parent.size += uint64(len(key)) + uint64(len(value))
	return true
}

func (self *sizeVisitor) getNode(path string) *sizeNode {
	if path == "" {
		return self.getNode("/")
	}
	node, found := self.m[path]
	if !found {
		node = &sizeNode{
			path: path,
		}
		self.m[path] = node
	}
	return node
}
