package api

import (
	"fmt"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	random "math/rand"
	"strconv"
	"strings"

	"github.com/pierrec/lz4"
)

func CompressData(ssource string) []byte {
	source := []byte(ssource)
	compressed := make([]byte, len(source))
	_, err := lz4.CompressBlockHC(source, compressed, 0)
	if err != nil {
		return source
	}
	compressed, err = trimNullBytes(compressed)
	if err != nil {
		return source
	}
	return compressed
}

func DecompressData(source []byte) string {
	decompressed := make([]byte, len(source) * 10)
	_, err := lz4.UncompressBlock(source, decompressed)
	if err != nil {
		return string(source)
	}
	decompressedTrimmed, err := trimNullBytes(decompressed)
	if err != nil {
		return string(decompressed)
	}
	return string(decompressedTrimmed)
}

func GenRandomString(n int) string {
	letters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[random.Intn(len(letters))]
	}
	return string(b)
}

func appendByte(slice []byte, data ...byte) []byte {
	m := len(slice)
	n := m + len(data)
	if n > cap(slice) {
		newSlice := make([]byte, (n + 1) * 2)
		copy(newSlice, slice)
		slice = newSlice
	}
	slice = slice[0:n]
	copy(slice[m:n], data)
	return slice
}

func trimNullBytes(slice []byte) ([]byte, error) {
	l := len(slice)
	l -= 1
	for i := l; i >= 0; i-- {
		if slice[i] != 0 {
			return slice[:i+1], nil
		}
	}
	return slice, errors.New("Can't trim null bytes")
}

func stringifySlice(slice []byte) string {
	result := strconv.Itoa(int(slice[0]))
	for i := 1; i < len(slice); i++ {
		result = fmt.Sprintf("%s %d", result, int(slice[i]))
	}
	return result
}

func bytifyString(str string) []byte {
	var result []byte
	slice := strings.Split(str, " ")
	for i := 0; i < len(slice); i++ {
		numbered, _ := strconv.Atoi(slice[i])
		result = appendByte(result, byte(numbered))
	}
	return result
}

func Hexify(source interface{}) string {
	if str, ok := source.(string); ok {
    return hex.EncodeToString([]byte(str))
	}
	b, _ := source.([]byte)
	return hex.EncodeToString(b)
}

func Dehexify(source string) ([]byte, error) {
	result, err := hex.DecodeString(source)
	return []byte(result), err
}

func parseCurrentCipher(path string, receiver string) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errors.New("can't parse file")
	}
	for _, line := range strings.Split(string(data), "\n") {
		split := strings.Split(line, "*:*")
		if split[1] == receiver {
			return split[2], nil
		}
	}
	return "", errors.New("receiver not found")
}

func (c *Commander) CipherMessage(receiver string, msg string) []byte {
	realPath := c.ConstantPath + "/history/history"
	// now all the encryption works only with byte slices
	bytedMessage := []byte(msg)
	// b represents message
	b := base64.StdEncoding.EncodeToString(bytedMessage)
	rblock, _ := GetRandomBlock()
	randomCipher := rblock.hash
	number := rblock.number
	strNumber := strconv.Itoa(number)
	// n represents blockchain's block number
	n := base64.StdEncoding.EncodeToString([]byte(strNumber))
	decodedRandomCipher, _ := Dehexify(randomCipher)
	randomBlock, _ := aes.NewCipher(decodedRandomCipher)
	// parsing our database to get correct cipher from there
	constCipher, _ := parseCurrentCipher(realPath, receiver)
	decodedCipher, _ := Dehexify(constCipher)
	constBlock, _ := aes.NewCipher(decodedCipher)
	// creating variable that will contain encrypted number
	ciphernumber := make([]byte, aes.BlockSize+len(n))
	// initializing IV
	iv := ciphernumber[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return []byte{}
	}
	cfb := cipher.NewCFBEncrypter(constBlock, iv)
	cfb.XORKeyStream(ciphernumber[aes.BlockSize:], []byte(n))
	// creating variable that will contain encrypted message
	ciphertext := make([]byte, aes.BlockSize+len(b))
	// initializing IV
	iv = ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return []byte{}
	}
	cfb = cipher.NewCFBEncrypter(randomBlock, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))
	// now concat both slices into one long slice separated with three bytes
	result := append(ciphernumber, byte(42), byte(58), byte(42))
	for i := 0; i < len(ciphertext); i++ {
		result = append(result, ciphertext[i])
	}
	// returning encrypted message
	return result
}

func (c *Commander) DecipherMessage(receiver string, msg []byte) []byte {
	strMsg := stringifySlice(msg)
	split := strings.Split(strMsg, " 42 58 42 ")
	num := bytifyString(split[0])
	msg = bytifyString(split[1])
	realPath := c.ConstantPath + "/history/history"
	// parsing our database to get correct cipher from there
	constCipher, _ := parseCurrentCipher(realPath, receiver)
	decodedCipher, _ := Dehexify(constCipher)
	block, _ := aes.NewCipher(decodedCipher)
	if len(num) < aes.BlockSize {
		return []byte{}
	}
	iv := num[:aes.BlockSize]
	num = num[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(num, num)
	data, err := base64.StdEncoding.DecodeString(string(num))
	if err != nil {
		return []byte{}
	}
	blockNumber, err := strconv.Atoi(string(data))
	if err != nil {
		return []byte{}
	}
	hash, err := GetBlockHash(int64(blockNumber))
	if err != nil {
		return []byte{}
	}
	decodedCipher, err = Dehexify(hash)
	if err != nil {
		return []byte{}
	}
	block, err = aes.NewCipher(decodedCipher)
	if err != nil {
		return []byte{}
	}
	if len(msg) < aes.BlockSize {
		return []byte{}
	}
	iv = msg[:aes.BlockSize]
	msg = msg[aes.BlockSize:]
	cfb = cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(msg, msg)
	data, err = base64.StdEncoding.DecodeString(string(msg))
	if err != nil {
		return []byte{}
	}
	return data
}
