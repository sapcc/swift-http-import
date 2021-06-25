package util

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/sapcc/go-bits/logg"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
	"golang.org/x/crypto/openpgp/packet"
)

//GPGKeyRing contains a list of openpgp Entities. It is used to verify different
//types of GPG signatures.
type GPGKeyRing struct {
	EntityList openpgp.EntityList
	Mux        sync.RWMutex
}

//VerifyClearSignedGPGSignature takes a clear signed message and a GPGKeyRing to check
//if the signature is valid.
//If the key ring does not contain the concerning public key then the key is downloaded
//from a pool server and added to the existing key ring.
//A non-nil error is returned, if signature verification was unsuccessful.
func VerifyClearSignedGPGSignature(keyring *GPGKeyRing, messageWithSignature []byte) error {
	block, _ := clearsign.Decode(messageWithSignature)
	return verifyGPGSignature(keyring, block.Bytes, block.ArmoredSignature)
}

//VerifyDetachedGPGSignature takes a message, a detached signature, and a GPGKeyRing to check
//if the signature is valid. The detached signature is expected to be armored.
//If the key ring does not contain the concerning public key then the key is downloaded
//from a pool server and added to the existing key ring.
//A non-nil error is returned, if signature verification was unsuccessful.
func VerifyDetachedGPGSignature(keyring *GPGKeyRing, message, armoredSignature []byte) error {
	block, err := armor.Decode(bytes.NewReader(armoredSignature))
	if err != nil {
		return err
	}
	return verifyGPGSignature(keyring, message, block)
}

func verifyGPGSignature(keyring *GPGKeyRing, message []byte, signature *armor.Block) error {
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

		//only download the public key if not found in the existing key ring
		keyring.Mux.RLock()
		foundKeys := keyring.EntityList.KeysById(issuerKeyID)
		keyring.Mux.RUnlock()
		if len(foundKeys) == 0 {
			b, err := getPublicKey(fmt.Sprintf("%X", issuerKeyID))
			if err != nil {
				return err
			}
			publicKeyBytes = append(publicKeyBytes, b...)
		}
	}

	//add the downloaded keys to the existing key ring
	if len(publicKeyBytes) != 0 {
		el, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(publicKeyBytes))
		if err != nil {
			return err
		}
		keyring.Mux.Lock()
		keyring.EntityList = append(keyring.EntityList, el...)
		keyring.Mux.Unlock()
	}

	keyring.Mux.RLock()
	_, err = openpgp.CheckDetachedSignature(keyring.EntityList, bytes.NewReader(message), bytes.NewReader(signatureBytes))
	keyring.Mux.RUnlock()

	return err
}

func getPublicKey(id string) ([]byte, error) {
	logg.Info("retrieving public key for ID %q", id)

	keyserverURLPatterns := strings.Split(os.Getenv("SHI_KEYSERVER_URLS"), " ")
	if len(keyserverURLPatterns) == 1 && keyserverURLPatterns[0] == "" {
		keyserverURLPatterns = []string{"https://pgp.mit.edu/pks/lookup?search=0x{keyid}&options=mr&op=get"}
	}

	for idx, keyserverURLPattern := range keyserverURLPatterns {
		url := strings.Replace(keyserverURLPattern, "{keyid}", id, -1)
		buf, err := getPublicKeyFromServer(id, url)
		if err == nil {
			return buf, nil
		}

		if idx == len(keyserverURLPatterns)-1 {
			logg.Error("could not retrieve public key for ID %q from %s: %s (no more servers to try)", id, url, err.Error())
		} else {
			logg.Error("could not retrieve public key for ID %q from %s: %s (will try next server)", id, url, err.Error())
		}
	}

	return nil, errNoSuchPublicKey
}

var errNoSuchPublicKey = errors.New("no such public key")

func getPublicKeyFromServer(id string, uri string) ([]byte, error) {
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(string(b)), "no results found") {
		return nil, errNoSuchPublicKey
	}

	return b, nil
}
