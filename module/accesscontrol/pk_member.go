/*
Copyright (C) BABEC. All rights reserved.
Copyright (C) THL A29 Limited, a Tencent company. All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"encoding/hex"
	"fmt"

	bccrypto "chainmaker.org/chainmaker/common/v2/crypto"
	"chainmaker.org/chainmaker/common/v2/crypto/asym"
	pbac "chainmaker.org/chainmaker/pb-go/v2/accesscontrol"
	"chainmaker.org/chainmaker/protocol/v2"
	commonCert "chainmaker.org/chainmaker/common/v2/cert"
)

var _ protocol.Member = (*pkMember)(nil)

// an instance whose member type is a certificate
type pkMember struct {

	// pem public key
	id string

	// organization identity who owns this member
	orgId string

	// public key uid
	uid string

	// the public key used for authentication
	pk bccrypto.PublicKey

	// role of this member
	role protocol.Role

	// hash type from chain configuration
	hashType string
}

func (pm *pkMember) GetMemberId() string {
	return pm.id
}

func (pm *pkMember) GetOrgId() string {
	return pm.orgId
}

func (pm *pkMember) GetRole() protocol.Role {
	return pm.role
}

func (pm *pkMember) GetUid() string {
	return pm.uid
}

func (pm *pkMember) Verify(hashType string, msg []byte, sig []byte) error {

	hash, ok := bccrypto.HashAlgoMap[hashType]
	if !ok {
		return fmt.Errorf("cert member verify signature failed: unsupport hash type")
	}
	ok, err := pm.pk.VerifyWithOpts(msg, sig, &bccrypto.SignOpts{
		Hash: hash,
		UID:  bccrypto.CRYPTO_DEFAULT_UID,
	})
	if err != nil {
		return fmt.Errorf("cert member verify signature failed: [%s]", err.Error())
	}
	if !ok {
		return fmt.Errorf("cert member verify signature failed: invalid signature")
	}
	return nil
}

func (pm *pkMember) GetMember() (*pbac.Member, error) {
	memberInfo, err := pm.pk.String()
	if err != nil {
		return nil, fmt.Errorf("get pb member failed: %s", err.Error())
	}
	return &pbac.Member{
		OrgId:      pm.orgId,
		MemberInfo: []byte(memberInfo),
		MemberType: pbac.MemberType_PUBLIC_KEY,
	}, nil
}

type signingPkMember struct {
	// Extends Identity
	pkMember

	// Sign the message
	sk bccrypto.PrivateKey
}

// When using public key instead of certificate, hashType is used to specify the hash algorithm while the signature algorithm is decided by the public key itself.
func (spm *signingPkMember) Sign(hashType string, msg []byte) ([]byte, error) {
	hash, ok := bccrypto.HashAlgoMap[hashType]
	if !ok {
		return nil, fmt.Errorf("sign failed: unsupport hash type")
	}
	return spm.sk.SignWithOpts(msg, &bccrypto.SignOpts{
		Hash: hash,
		UID:  bccrypto.CRYPTO_DEFAULT_UID,
	})
}

func NewPkMember(member *pbac.Member, acs *accessControlService) (*pkMember, error) {
	//if member.MemberType != pbac.MemberType_PUBLIC_KEY {
	//	return nil, fmt.Errorf("setup public key member failed, unsupport member type")
	//} else {
	//	return newMemberFromPkPem(member.OrgId, string(member.MemberInfo), acs.hashType)
	//}
}

func newMemberFromPkPem(orgId, role, pkPEM string, hashType string) (*pkMember, error) {

	hash, ok := bccrypto.HashAlgoMap[hashType]
	if !ok {
		return nil, fmt.Errorf("sign failed: unsupport hash type")
	}

	var pkMember pkMember
	pkMember.orgId = orgId
	pkMember.hashType = hashType

	pk, err := asym.PublicKeyFromPEM([]byte(pkPEM))
	if err != nil {
		return nil, fmt.Errorf("setup pk member failed, err: %s", err.Error())
	}

	pkMember.pk = pk
	pkMember.id = pkPEM
	pkMember.role = protocol.Role(role)
	ski, err := commonCert.ComputeSKI(hash, pk.ToStandardKey())

	if err != nil {
		return nil, fmt.Errorf("setup pk member failed, err: %s", err.Error())
	}

	pkMember.uid = hex.EncodeToString(ski)

	return &pkMember, nil
}
