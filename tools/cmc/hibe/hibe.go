package hibe

import (
	localhibe "chainmaker.org/chainmaker-go/common/crypto/hibe"
	"errors"
	"fmt"
	"github.com/samkumar/hibe"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"vuvuzela.io/crypto/bn256"
	"vuvuzela.io/crypto/rand"
)

var (
	savePathSuffix string

	// initHibeCMD flags
	level string
	path  string
	orgId string

	// getParamsCMD flags

	// genPrvKeyCMD flags
	paramsFilePath     string
	fromMaster         int
	keyFilePath        string
	privateKeySavePath string
	id                 string

	// updatePrvKeyCMD flags
)

func HibeCMD() *cobra.Command {
	hibeCmd := &cobra.Command{
		Use:   "hibe",
		Short: "ChainMaker hibe command",
		Long:  "ChainMaker hibe command",
	}
	hibeCmd.AddCommand(initHibeCMD())
	hibeCmd.AddCommand(getParamsCMD())
	hibeCmd.AddCommand(genPrvKeyCMD())
	//hibeCmd.AddCommand(updatePrvKeyCMD())
	return hibeCmd
}

func initHibeCMD() *cobra.Command {
	setupHibeCmd := &cobra.Command{
		Use:   "init",
		Short: "setup generates the system parameters",
		Long:  "setup generates the system parameters",
		RunE: func(_ *cobra.Command, _ []string) error {
			return setupOrgHibeSys()
		},
	}

	flags := setupHibeCmd.Flags()
	flags.StringVarP(&level, "level", "l", "", "the parameter \"l\" is the maxi depth that the hierarchy will support.")
	flags.StringVarP(&path, "spath", "s", "", "the result storage path, include org's params、MasterKey")
	flags.StringVarP(&orgId, "orgId", "o", "", "the result storage name, please enter your orgId")

	return setupHibeCmd
}

func getParamsCMD() *cobra.Command {
	getParamsCmd := &cobra.Command{
		Use:   "getParams",
		Short: "getParams storage path ",
		Long:  "getParams storage path ",
		RunE: func(_ *cobra.Command, _ []string) error {
			return getParams()
		},
	}

	flags := getParamsCmd.Flags()
	flags.StringVarP(&orgId, "orgId", "o", "", "the result storage name, please enter your orgId")
	flags.StringVarP(&path, "path", "p", "", "the init path")

	return getParamsCmd
}

func genPrvKeyCMD() *cobra.Command {
	genPrivateKeyCmd := &cobra.Command{
		Use:   "genPrvKey",
		Short: "generates a key for an Id using the master key",
		Long:  "generates a key for an Id using the master key",
		RunE: func(_ *cobra.Command, _ []string) error {
			return genPrivateKey()
		},
	}

	flags := genPrivateKeyCmd.Flags()
	flags.StringVarP(&paramsFilePath, "ppath", "p", "", "the hibe params file's path")
	flags.IntVarP(&fromMaster, "fromMaster", "m", 0, "generate prvKey from masterKey or privateKey, 1 from master, 0 from parent, m default is 0")
	flags.StringVarP(&keyFilePath, "kpath", "k", "", "the masterKey Or parentKey file path")
	flags.StringVarP(&privateKeySavePath, "spath", "s", "", "the result storage file path, and the file name is the id")
	flags.StringVarP(&id, "id", "i", "", "get the private key of the ID, Must be formatted in the sample format with\" / \", "+
		"for example: id org1/ou1/Alice")
	flags.StringVarP(&orgId, "orgId", "o", "", "the result storage name, please enter your orgId")

	return genPrivateKeyCmd
}

/*
func updatePrvKeyCMD() *cobra.Command {
	updatePrivateKeyCmd := &cobra.Command{
		Use:   "updatePrvKey",
		Short: "update a privateKey for an Id using the master or parent key",
		Long:  "update a privateKey for an Id using the master or parent key",
		RunE: func(_ *cobra.Command, _ []string) error {
			return genPrivateKey()
		},
	}

	flags := updatePrivateKeyCmd.Flags()
	flags.StringVarP(&paramsFilePath, "ppath", "p", "", "the parameter file's path")
	flags.IntVarP(&fromMaster, "fromMaster", "m", 0, "update prvKey from masterKey or privateKey, 1 from master, 0 from parent")
	flags.StringVarP(&keyFilePath, "kpath", "k", "", "the masterKey Or parentKey file path")
	flags.StringVarP(&privateKeySavePath, "spath", "s", "", "the privateKey storage file path, and the file name is the id")
	flags.StringVarP(&id, "id", "i", "", "update the private key of the ID, Must be formatted in the sample format with\" / \", "+
		"for example: id org1/ou1/Alice")

	return updatePrivateKeyCmd
}
*/

func setupOrgHibeSys() error {

	err := localhibe.ValidateId(orgId)
	if err != nil {
		return err
	}

	savePathSuffix = orgId

	l, err := strconv.Atoi(level)
	if err != nil {
		return errors.New("invalid parameter, level supports integers from 1 to 10")
	}

	if l < 1 || l > 10 {
		return errors.New("invalid parameter, level supports integers from 1 to 10")
	}

	if path == "" {
		return errors.New("invalid parameter, the path cannot be empty")
	}

	path = filepath.Join(path, savePathSuffix)

	filePath := filepath.Join(path, fmt.Sprintf("%s.masterKey", orgId))
	exist, err := pathExists(filePath)
	if err != nil {
		return err
	}
	if exist {
		return fmt.Errorf("file [ %s ] already exist", filePath)
	}

	params, masterKey, err := hibe.Setup(rand.Reader, l)

	if err = os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("mk hibe dir failed, %s", err.Error())
	}

	if err = ioutil.WriteFile(filepath.Join(path, fmt.Sprintf("%s.params", orgId)),
		params.Marshal(), 0600); err != nil {
		return fmt.Errorf("save hibe params to file [%s] failed, %s", path, err.Error())
	}
	fmt.Printf("[%s params] storage file path: %s\n", orgId, filepath.Join(path, fmt.Sprintf("%s.params", orgId)))

	if err = ioutil.WriteFile(filePath, (*masterKey).Marshal(), 0600); err != nil {
		return fmt.Errorf("save hibe params to file [%s] failed, %s", path, err.Error())
	}
	fmt.Printf("[%s masterKey] storage file path: %s\n", orgId, filepath.Join(path, fmt.Sprintf("%s.masterKey", orgId)))

	return nil
}

func getParams() error {
	if err := localhibe.ValidateId(orgId); err != nil {
		return err
	}

	savePathSuffix = orgId

	if path == "" {
		return errors.New("invalid parameter, path cannot be empty")
	}

	path = filepath.Join(path, savePathSuffix)

	filePath := filepath.Join(path, fmt.Sprintf("%s.params", orgId))
	exist, err := pathExists(filePath)
	if err != nil {
		return err
	}
	if !exist {
		return fmt.Errorf("file [ %s ] does not exist", filePath)
	}

	paramsBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("open hibe params's file [%s] failed, %s", fmt.Sprintf("%s.params", orgId), err.Error())
	}

	params := new(hibe.Params)
	params, ok := params.Unmarshal(paramsBytes)
	if !ok {
		return fmt.Errorf("get params from [%s] failed, Unmarshal failed, please check file", fmt.Sprintf("%s.params", orgId))
	}

	fmt.Printf("[%s params] file path: %s\n", orgId, filepath.Join(path, fmt.Sprintf("%s.params", orgId)))
	fmt.Printf("[%s Params] : %+v\n", orgId, params)
	return nil
}

func genPrivateKey() error {
	err := localhibe.ValidateId(orgId)
	if err != nil {
		return err
	}

	savePathSuffix = orgId

	err = localhibe.ValidateId(id)
	if err != nil {
		return err
	}

	exist, err := pathExists(keyFilePath)
	if err != nil {
		return err
	}
	if !exist {
		return fmt.Errorf("file [ %s ] does not exists", keyFilePath)
	}

	strId, hibeId := localhibe.IdStr2HibeId(id)

	var fileName string
	for i, item := range strId {
		if i == 0 {
			fileName = fmt.Sprintf("%s%s", fileName, item)
		} else {
			fileName = fmt.Sprintf("%s#%s", fileName, item)
		}
	}

	dir := filepath.Join(privateKeySavePath, savePathSuffix, "privateKeys")
	fileName = fmt.Sprintf("%s.privateKey", fileName)
	filePath := filepath.Join(privateKeySavePath, savePathSuffix, "privateKeys", fileName)
	exist, err = pathExists(filePath)
	if err != nil {
		return err
	}
	if exist {
		return fmt.Errorf("file [ %s ] already exist", filePath)
	}

	exist, err = pathExists(paramsFilePath)
	if err != nil {
		return err
	}
	if !exist {
		return fmt.Errorf("file [ %s ] does not exist", paramsFilePath)
	}

	paramsBytes, err := ioutil.ReadFile(paramsFilePath)
	if err != nil {
		return fmt.Errorf("open file [%s] failed, %s", paramsFilePath, err.Error())
	}

	params := new(hibe.Params)
	params, ok := params.Unmarshal(paramsBytes)
	if !ok {
		return errors.New("params.Unmarshal() failed, please check your params file")
	}

	if len(strings.Split(id, "/")) > params.MaximumDepth() {
		return fmt.Errorf("invalid parameter, the max level is %d", params.MaximumDepth())
	}

	var privateKey *hibe.PrivateKey
	if fromMaster == 1 {
		masterKeyBytes, err := ioutil.ReadFile(keyFilePath)
		if err != nil {
			return fmt.Errorf("open file [%s] failed, %s", paramsFilePath, err.Error())
		}
		masterKey := new(bn256.G1)

		masterKey, ok = masterKey.Unmarshal(masterKeyBytes)
		if !ok {
			return errors.New("params.Unmarshal() failed, please check your masterKey file")
		}
		privateKey, err = hibe.KeyGenFromMaster(rand.Reader, params, masterKey, hibeId)
		if err != nil {
			return err
		}
	} else {
		// default key gen from parent
		pathSlice := strings.Split(keyFilePath, "/")
		parentFileName := pathSlice[len(pathSlice)-1]
		parentFileName = strings.TrimSuffix(parentFileName, ".privateKey")
		parentIdStr := strings.ReplaceAll(parentFileName, "#", "/")

		if !strings.HasPrefix(id, parentIdStr) {
			return fmt.Errorf("no permission, the input ID [ %s ] is not your subordinate level", id)
		}
		matchedId := id

		parentKeyBytes, err := ioutil.ReadFile(keyFilePath)
		if err != nil {
			return fmt.Errorf("open file [%s] failed, %s", keyFilePath, err.Error())
		}
		parentKey := new(hibe.PrivateKey)

		parentKey, ok = parentKey.Unmarshal(parentKeyBytes)
		if !ok {
			return errors.New("params.Unmarshal() failed, please check your privateKey file")
		}

		matchedIdStr, hibeIds := localhibe.IdStr2HibeId(matchedId)

		parentIdStrLen := len(strings.Split(parentIdStr, "/"))
		for i := parentIdStrLen + 1; i <= len(matchedIdStr); i++ {
			parentKey, err = hibe.KeyGenFromParent(rand.Reader, params, parentKey, hibeIds[:i])
			if err != nil {
				return err
			}
		}
		privateKey = parentKey
	}

	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("mk hibe privateKey dir failed, %s", err.Error())
	}

	if err = ioutil.WriteFile(filePath, privateKey.Marshal(), 0600); err != nil {
		return fmt.Errorf("save privateKey to file [%s] failed, %s", fileName, err.Error())
	}

	fmt.Printf("%s privateKey storage file path: %s/%s/privateKeys/%s\n", strId, privateKeySavePath, savePathSuffix, fileName)

	return nil
}

// pathExists is used to determine whether a file or folder exists
func pathExists(path string) (bool, error) {
	if path == "" {
		return false, errors.New("invalid parameter, the file path cannot be empty")
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
