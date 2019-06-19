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

//VerifyClearSignedGPGSignature takes a clear signed message and a keyring, and checks
//if the signature is valid.
//If the keyring does not contain the respective public key then the key is downloaded
//and added to the existing keyring.
//error is nil if signature verification was successful.
func VerifyClearSignedGPGSignature(keyring *openpgp.EntityList, messageWithSignature []byte) error {
	block, _ := clearsign.Decode(messageWithSignature)
	return verifyGPGSignature(keyring, block.Bytes, block.ArmoredSignature)
}

//VerifyDetachedGPGSignature takes a message, a detached signature, and a keyring to check
//if the signature is valid. The detached signature is expected to be armored.
//If the keyring does not contain the respective public key then the key is downloaded
//and added to the existing keyring.
//error is nil if signature verification was successful.
func VerifyDetachedGPGSignature(keyring *openpgp.EntityList, message, armoredSignature []byte) error {
	block, err := armor.Decode(bytes.NewReader(armoredSignature))
	if err != nil {
		return err
	}
	return verifyGPGSignature(keyring, message, block)
}

func verifyGPGSignature(keyring *openpgp.EntityList, message []byte, signature *armor.Block) error {
	if signature.Type != openpgp.SignatureType {
		return fmt.Errorf("invalid OpenPGP armored structure: expected %q, got %q", openpgp.SignatureType, signature.Type)
	}

	var publicKeyBytes []byte

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

		var issuerKeyID uint64
		switch t := pkt.(type) {
		default:
			return fmt.Errorf("invalid OpenPGP packet type: expected either %q or %q, got %T", "*packet.Signature", "*packet.SignatureV3", t)
		case *packet.Signature:
			issuerKeyID = *pkt.(*packet.Signature).IssuerKeyId
		case *packet.SignatureV3:
			issuerKeyID = pkt.(*packet.SignatureV3).IssuerKeyId
		}

		//only download the public key, if not found in the existing keyring
		foundKeys := (*keyring).KeysById(issuerKeyID)
		if len(foundKeys) == 0 {
			b, err := getPublicKey(fmt.Sprintf("%X", issuerKeyID))
			if err != nil {
				return err
			}
			publicKeyBytes = append(publicKeyBytes, b...)
		}
	}

	//add the downloaded keys to the existing keyring
	el, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(publicKeyBytes))
	if err != nil {
		return err
	}
	*keyring = append(*keyring, el...)

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
