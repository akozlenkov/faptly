package pgp

import (
	"bytes"
	"errors"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
)

func decodePrivateEntity(privateKey []byte, passphrase []byte) (*openpgp.Entity, error) {
	entityList, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(privateKey))
	if err != nil {
		return nil, err
	}
	if len(entityList) == 0 {
		return nil, errors.New("no keys found")
	}

	entity := entityList[0]
	if entity.PrivateKey.Encrypted {
		if err := entity.PrivateKey.Decrypt(passphrase); err != nil {
			return nil, err
		}
	}
	for _, sub := range entity.Subkeys {
		if sub.PrivateKey != nil && sub.PrivateKey.Encrypted {
			if err := sub.PrivateKey.Decrypt(passphrase); err != nil {
				return nil, err
			}
		}
	}

	return entity, nil
}

func SignData(privateKey []byte, passphrase []byte, data []byte) ([]byte, error) {
	signed := &bytes.Buffer{}

	entity, err := decodePrivateEntity(privateKey, passphrase)
	if err != nil {
		return nil, err
	}

	encoder, err := clearsign.Encode(signed, entity.PrivateKey, nil)
	if err != nil {
		return nil, err
	}

	if _, err := encoder.Write(data); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return signed.Bytes(), nil
}
