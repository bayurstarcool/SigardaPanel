package secrets

import "encoding/json"

// SensitiveKeys are JSON payload fields that hold credentials and must be
// encrypted at rest in the job queue.
var SensitiveKeys = map[string]bool{
	"access_key":  true,
	"secret_key":  true,
	"db_password": true,
	"password":    true,
	"secret":      true,
	"token":       true,
}

// EncryptPayload parses a JSON job payload and encrypts the string values of
// sensitive keys (recursively). Non-JSON input is returned unchanged. Values
// already encrypted are left as-is.
func EncryptPayload(payload string) (string, error) {
	return transformPayload(payload, encryptStr)
}

// DecryptPayload reverses EncryptPayload, decrypting sensitive-key values so
// the plaintext can be dispatched to an agent. Non-JSON input is returned
// unchanged.
func DecryptPayload(payload string) (string, error) {
	return transformPayload(payload, Decrypt)
}

func encryptStr(s string) (string, error) {
	if IsEncrypted(s) {
		return s, nil
	}
	return Encrypt(s)
}

func transformPayload(payload string, fn func(string) (string, error)) (string, error) {
	if payload == "" {
		return payload, nil
	}
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return payload, nil
	}
	out, err := walk(v, false, fn)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// walk recursively transforms string values whose key is sensitive. sensitive
// propagates to nested objects/arrays under a sensitive key (e.g. s3_config is
// not sensitive itself, but its access_key/secret_key children are matched by
// key name, so propagation is only used for arrays of secret-bearing objects).
func walk(v any, sensitive bool, fn func(string) (string, error)) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			child := SensitiveKeys[k]
			nv, err := walk(val, child, fn)
			if err != nil {
				return nil, err
			}
			t[k] = nv
		}
		return t, nil
	case []any:
		for i, val := range t {
			nv, err := walk(val, sensitive, fn)
			if err != nil {
				return nil, err
			}
			t[i] = nv
		}
		return t, nil
	case string:
		if sensitive {
			return fn(t)
		}
		return t, nil
	default:
		return v, nil
	}
}
