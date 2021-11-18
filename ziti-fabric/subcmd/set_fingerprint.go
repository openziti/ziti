package subcmd

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
	"regexp"
	"strings"
)

func init() {
	Root.AddCommand(setRouterFingerprintCmd)
}

var setRouterFingerprintCmd = &cobra.Command{
	Use:   "fingerprint <db-file> <router-id> <fingerprint>",
	Short: "Manually specify the router client fingerprint, the controller must not be running.",
	Args:  cobra.ExactArgs(3),
	Run:   setRouterFingerprint,
}

func setRouterFingerprint(cmd *cobra.Command, args []string) {
	dbFile := strings.TrimSpace(args[0])
	routerId := strings.TrimSpace(args[1])
	fingerprint := strings.TrimSpace(args[2])

	fingerprint = cleanHexString(fingerprint)

	if dbFile == "" {
		pfxlog.Logger().Fatalf("<db-file> must specified")
	}

	if routerId == "" {
		pfxlog.Logger().Fatalf("<router-id> must specified")
	}

	if fingerprint == "" {
		pfxlog.Logger().Fatalf("<fingerprint> must specified and be a valid hex characters")
	}

	db, err := bbolt.Open(dbFile, 0666, nil)

	if err != nil || db == nil {
		pfxlog.Logger().Fatalf("could not open database [%s]: %v", dbFile, err)
	}



	err = db.Update(func(tx *bbolt.Tx) error {
		zitiBucket := tx.Bucket([]byte("ziti"))

		if zitiBucket == nil {
			return fmt.Errorf("could not open 'ziti': %v", "not found")
		}

		routersBucket := zitiBucket.Bucket([]byte("routers"))

		if routersBucket == nil {
			return fmt.Errorf("could not open 'ziti.routers': %v", "not found")
		}

		targetRouterBucket := routersBucket.Bucket([]byte(routerId))

		if targetRouterBucket == nil {
			return fmt.Errorf("could not open 'ziti.routers.%s': %s", routerId, "not found")
		}

		value := boltz.PrependFieldType(boltz.TypeString, []byte(fingerprint))
		err = targetRouterBucket.Put([]byte("fingerprint"), value)

		if err != nil {
			return fmt.Errorf("could not put ziti.routers.%s.fingerprint: %v", routerId, err)
		}

		return nil
	})

	if err != nil {
		pfxlog.Logger().Fatalf("could not update router: %v", err)
		return
	}

	pfxlog.Logger().Infof("done")
}


func cleanHexString(in string) string {
	hexClean := regexp.MustCompile("[^a-f0-9]+")
	return hexClean.ReplaceAllString(strings.ToLower(in), "")
}