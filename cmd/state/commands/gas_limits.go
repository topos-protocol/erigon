package commands

import (
	"fmt"

	"github.com/ledgerwatch/erigon/common/dbutils"

	"github.com/ledgerwatch/erigon/cmd/state/stateless"
	"github.com/ledgerwatch/erigon/ethdb"
	"github.com/spf13/cobra"
)

func init() {
	withPrivateApi(gasLimitsCmd)
	rootCmd.AddCommand(gasLimitsCmd)
}

var gasLimitsCmd = &cobra.Command{
	Use:   "gasLimits",
	Short: "gasLimits",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		localDB := ethdb.NewLMDB().Path(file() + "_gl").WithBucketsConfig(func(defaultBuckets dbutils.BucketsCfg) dbutils.BucketsCfg {
			return dbutils.BucketsCfg{
				stateless.MainHashesBucket:      {},
				stateless.ReportsProgressBucket: {},
			}
		}).MustOpen()

		remoteDB, err := ethdb.NewRemote().Path(privateApiAddr).Open("", "", "")
		if err != nil {
			return err
		}

		fmt.Println("Processing started...")
		stateless.NewGasLimitReporter(ctx, remoteDB, localDB).GasLimits(ctx)
		return nil
	},
}
