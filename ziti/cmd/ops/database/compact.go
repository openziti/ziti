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
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
	"math"
)

type CompactAction struct {
	useArrayFreelists bool
}

func NewCompactAction() *cobra.Command {
	action := &CompactAction{}

	cmd := &cobra.Command{
		Use:   "compact <src> <dest>",
		Short: "Compact a bbolt database",
		Args:  cobra.ExactArgs(2),
		RunE:  action.Run,
	}

	cmd.Flags().BoolVar(&action.useArrayFreelists, "array-freelist", true, "Use array freelist")
	return cmd
}

// Run implements this command
func (o *CompactAction) Run(cmd *cobra.Command, args []string) error {
	srcOptions := *bbolt.DefaultOptions
	srcOptions.ReadOnly = true

	srcDb, err := bbolt.Open(args[0], 0400, &srcOptions)
	if err != nil {
		return err
	}

	dstOptions := *bbolt.DefaultOptions
	if !o.useArrayFreelists {
		dstOptions.FreelistType = bbolt.FreelistMapType
	}
	dstDb, err := bbolt.Open(args[1], 0600, &dstOptions)
	if err != nil {
		return err
	}

	return bbolt.Compact(dstDb, srcDb, math.MaxUint16)
}
