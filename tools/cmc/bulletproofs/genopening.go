package bulletproofs

import (
	"chainmaker.org/chainmaker/common/crypto/bulletproofs"
	"encoding/base64"
	"fmt"
	"github.com/spf13/cobra"
)

func genOpeningCMD() *cobra.Command {
	genOpeningCmd := &cobra.Command{
		Use:   "genOpening",
		Short: "Bulletproofs generate opening command",
		Long:  "Bulletproofs generate opening command",
		RunE: func(_ *cobra.Command, _ []string) error {
			return genOpening()
		},
	}

	return genOpeningCmd
}

func genOpening() error {
	opening, err := bulletproofs.Helper().NewBulletproofs().PedersenRNG()
	if err != nil {
		return err
	}

	openingStr := base64.StdEncoding.EncodeToString(opening)
	fmt.Printf("opening: [%s]\n", openingStr)

	return nil
}
