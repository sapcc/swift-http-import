package util

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
	"golang.org/x/crypto/openpgp/packet"
)

//VerifyClearSignedGPGSignature takes a clear signed message and and checks
//if the signature is valid.
//error is nil if signature verification was successful.
func VerifyClearSignedGPGSignature(messageWithSignature []byte) error {
	block, _ := clearsign.Decode(messageWithSignature)
	return verifyGPGSignature(block.Bytes, block.ArmoredSignature)
}

// VerifyDetachedGPGSignature takes a message and a detached signature, and checks
// if the signature is valid. The detached signature is expected to be armored.
// error is nil if signature verification was successful.
func VerifyDetachedGPGSignature(message, armoredSignature []byte) error {
	block, err := armor.Decode(bytes.NewReader(armoredSignature))
	if err != nil {
		return err
	}
	return verifyGPGSignature(message, block)
}

func verifyGPGSignature(message []byte, signature *armor.Block) error {
	if signature.Type != openpgp.SignatureType {
		return fmt.Errorf("invalid OpenPGP armored structure: expected %q, got %q", openpgp.SignatureType, signature.Type)
	}

	var publicKeys []byte

	signatureBytes, err := ioutil.ReadAll(signature.Body)
	if err != nil {
		return err
	}
	r := packet.NewReader(bytes.NewReader(signatureBytes))
	for {
		pkt, err := r.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		var issuerKeyID string
		switch t := pkt.(type) {
		default:
			return fmt.Errorf("invalid OpenPGP packet type: expected either %q or %q, got %T", "*packet.Signature", "*packet.SignatureV3", t)
		case *packet.Signature:
			issuerKeyID = fmt.Sprintf("%X", *pkt.(*packet.Signature).IssuerKeyId)
		case *packet.SignatureV3:
			issuerKeyID = fmt.Sprintf("%X", pkt.(*packet.SignatureV3).IssuerKeyId)
		}

		b, err := getPublicKey(issuerKeyID)
		if err != nil {
			return err
		}

		publicKeys = append(publicKeys, b...)
	}

	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(publicKeys))
	if err != nil {
		return err
	}

	_, err = openpgp.CheckDetachedSignature(keyring, bytes.NewReader(message), bytes.NewReader(signatureBytes))
	return err
}

func getPublicKey(id string) ([]byte, error) {
	uri := fmt.Sprintf("pool.sks-keyservers.net/pks/lookup?search=0x%s&options=mr&op=get", id)

	// try different mirrors in case of failure
	resp, err := http.Get("http://" + "hkps." + uri)
	if err != nil {
		resp, err = http.Get("http://" + "eu." + uri)
		if err != nil {
			resp, err = http.Get("http://" + "na." + uri)
			if err != nil {
				resp, err = http.Get("http://" + uri)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(string(b)), "no results found") {
		return nil, fmt.Errorf("no public key found for %q", id)
	}

	return b, nil
}
