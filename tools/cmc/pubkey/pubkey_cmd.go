/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pubkey

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"chainmaker.org/chainmaker-go/tools/cmc/util"
	"chainmaker.org/chainmaker/common/v2/crypto"

	"chainmaker.org/chainmaker/pb-go/v2/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	sdk "chainmaker.org/chainmaker/sdk-go/v2"
	sdkutils "chainmaker.org/chainmaker/sdk-go/v2/utils"
)

var (
	pubkeyFile string
	orgId      string
	role       string
)

var (
	sdkConfPath        string
	clientKeyFilePaths string // nolint: deadcode, varcheck, unused
	chainId            string
	adminKeyFilePaths  string
	adminOrgIds        string
)

const (
	flagSdkConfPath        = "sdk-conf-path"
	flagClientKeyFilePaths = "client-key-file-paths" // nolint: deadcode, varcheck
	flagChainId            = "chain-id"
	flagAdminKeyFilePaths  = "admin-key-file-paths"
	flagAdminOrgIds        = "admin-org-ids"

	flagPubkeyFilePath = "pubkey-file-path"
	flagOrgId          = "org-id"
	flagRole           = "role"
)

func NewPubkeyCMD() *cobra.Command {
	pkCmd := &cobra.Command{
		Use:   "pubkey",
		Short: "pk management command.",
		Long:  "public key management command.",
	}

	pkCmd.PersistentFlags().StringVar(&sdkConfPath, flagSdkConfPath, "",
		"specify sdk config path")
	pkCmd.PersistentFlags().StringVar(&chainId, flagChainId, "",
		"specify the chain id, such as: chain1, chain2 etc.")
	pkCmd.PersistentFlags().StringVar(&adminKeyFilePaths, flagAdminKeyFilePaths, "",
		"specify admin key file paths, use ',' to separate")
	pkCmd.PersistentFlags().StringVar(&adminOrgIds, flagAdminOrgIds, "",
		"specify admin org-ids, use ',' to separate")

	pkCmd.MarkPersistentFlagRequired(flagSdkConfPath)
	pkCmd.MarkPersistentFlagRequired(flagChainId)
	pkCmd.MarkPersistentFlagRequired(flagAdminKeyFilePaths)
	pkCmd.MarkPersistentFlagRequired(flagAdminOrgIds)

	pkCmd.AddCommand(AddPKCmd())
	pkCmd.AddCommand(DelPKCmd())

	return pkCmd
}

func AddPKCmd() *cobra.Command {
	addPKCmd := &cobra.Command{
		Use:   "add",
		Long:  "add pubkey info.",
		Short: "add pubkey info.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return cliAddPubKey()
		},
	}

	flags := &pflag.FlagSet{}
	flags.StringVar(&pubkeyFile, flagPubkeyFilePath, "", "specify pubkey filename")
	flags.StringVar(&orgId, flagOrgId, "", "specify the orgId, such as wx-org1.chainmaker.com")
	flags.StringVar(&role, flagRole, "", "specify the role, such as client")

	addPKCmd.Flags().AddFlagSet(flags)

	addPKCmd.MarkFlagRequired(flagPubkeyFilePath)
	addPKCmd.MarkFlagRequired(flagOrgId)
	addPKCmd.MarkFlagRequired(flagRole)

	return addPKCmd
}

func DelPKCmd() *cobra.Command {
	delPKCmd := &cobra.Command{
		Use:   "del",
		Long:  "del pubkey info.",
		Short: "del pubkey info.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return cliDelPubKey()
		},
	}

	flags := &pflag.FlagSet{}
	flags.StringVar(&pubkeyFile, flagPubkeyFilePath, "", "specify pubkey filename")
	flags.StringVar(&orgId, flagOrgId, "", "specify the orgId, such as wx-org1.chainmaker.com")

	delPKCmd.Flags().AddFlagSet(flags)

	delPKCmd.MarkFlagRequired(flagPubkeyFilePath)
	delPKCmd.MarkFlagRequired(flagOrgId)

	return delPKCmd
}

func cliAddPubKey() error {
	adminKeys, adminOrgs, err := createMultiSignAdmins(adminKeyFilePaths, adminOrgIds)
	if err != nil {
		return err
	}

	file, err := os.Open(pubkeyFile)
	if err != nil {
		return fmt.Errorf("open file '%s' error: %v", pubkeyFile, err)
	}
	defer file.Close()

	pubkeyData, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read file '%v' error: %v", pubkeyFile, err)
	}

	client, err := CreateClientWithConfig()
	if err != nil {
		return fmt.Errorf("create user client failed, %s", err.Error())
	}
	defer client.Stop()

	err = client.CheckNewBlockChainConfig()
	if err != nil {
		return fmt.Errorf("check new blockchains failed, %s", err.Error())
	}

	payload, err := client.CreatePubkeyAddPayload(string(pubkeyData), orgId, role)
	if err != nil {
		return fmt.Errorf("save enclave ca cert failed, %s", err.Error())
	}

	endorsementEntrys := make([]*common.EndorsementEntry, len(adminKeys))
	for i := range adminKeys {
		e, err := sdkutils.MakePkEndorserWithPath(
			adminKeys[i],
			crypto.HashAlgoMap[client.GetHashType()],
			adminOrgs[i],
			payload,
		)
		if err != nil {
			return err
		}
		endorsementEntrys[i] = e
	}

	resp, err := client.SendPubkeyManageRequest(payload, endorsementEntrys, 5, false)
	if err != nil {
		return err
	}
	err = util.CheckProposalRequestResp(resp, false)
	if err != nil {
		return err
	}

	fmt.Println("command execute successfully.")
	return nil
}

func cliDelPubKey() error {
	adminKeys, adminOrgs, err := createMultiSignAdmins(adminKeyFilePaths, adminOrgIds)
	if err != nil {
		return err
	}

	file, err := os.Open(pubkeyFile)
	if err != nil {
		return fmt.Errorf("open file '%s' error: %v", pubkeyFile, err)
	}
	defer file.Close()

	pubkeyData, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read file '%v' error: %v", pubkeyFile, err)
	}

	client, err := CreateClientWithConfig()
	if err != nil {
		return fmt.Errorf("create user client failed, %s", err.Error())
	}
	defer client.Stop()

	err = client.CheckNewBlockChainConfig()
	if err != nil {
		return fmt.Errorf("check new blockchains failed, %s", err.Error())
	}

	payload, err := client.CreatePubkeyDelPayload(string(pubkeyData), orgId)
	if err != nil {
		return fmt.Errorf("save enclave ca cert failed, %s", err.Error())
	}

	endorsementEntrys := make([]*common.EndorsementEntry, len(adminKeys))
	for i := range adminKeys {
		e, err := sdkutils.MakePkEndorserWithPath(
			adminKeys[i],
			crypto.HashAlgoMap[client.GetHashType()],
			adminOrgs[i],
			payload,
		)
		if err != nil {
			return err
		}
		endorsementEntrys[i] = e
	}

	resp, err := client.SendPubkeyManageRequest(payload, endorsementEntrys, 5, false)
	if err != nil {
		return err
	}
	err = util.CheckProposalRequestResp(resp, false)
	if err != nil {
		return err
	}

	fmt.Println("command execute successfully.")
	return nil
}

func CreateClientWithConfig() (*sdk.ChainClient, error) {
	chainClient, err := sdk.NewChainClient(sdk.WithConfPath(sdkConfPath),
		sdk.WithChainClientOrgId(orgId), sdk.WithChainClientChainId(chainId))
	if err != nil {
		return nil, err
	}

	return chainClient, nil
}

func createMultiSignAdmins(adminKeyFilePaths string, adminOrgIds string) ([]string, []string, error) {
	adminKeys := strings.Split(adminKeyFilePaths, ",")
	adminOrgs := strings.Split(adminOrgIds, ",")
	if len(adminKeys) == 0 || len(adminOrgs) == 0 {
		return nil, nil, errors.New("no admin users given for sign payload")
	}
	if len(adminKeys) != len(adminOrgs) {
		return nil, nil, fmt.Errorf("admin keys num(%v) is not equals org-id num(%v)", len(adminKeys), len(adminOrgs))
	}

	return adminKeys, adminOrgs, nil
}
