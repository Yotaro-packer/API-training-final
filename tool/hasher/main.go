package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"os"
)

func main() {
	hash, err := ReadUserInput()
	if err != nil {
		fmt.Println(err)
	} else {
		msg := fmt.Sprintf("password hash: %s\n", hash)
		fmt.Print(msg)
		file, err := os.Create("hash.txt")
		// エラーチェック
		if err != nil {
			fmt.Println("ファイル作成エラー:", err)
			return
		}
		// ファイルに文字列を書き込み
		_, err = file.WriteString(msg)
		if err != nil {
			fmt.Println("書き込みエラー:", err)
		}
		// ファイルをクローズしてリソースを解放
		err = file.Close()
		if err != nil {
			fmt.Println("ファイルクローズエラー:", err)
		}
	}
}

func ReadUserInput() ([]byte, error) {
	fmt.Print("Passwordを入力してください: ")
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, nil
	}

	password := string(bytePassword)
	// hash the password
	hash, err := PasswordHash(bytePassword)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	//check if matches
	passwordMatch := HashPasswordCheck([]byte(password), hash)
	if !passwordMatch {
		return nil, fmt.Errorf("使用できない入力がありました\n")
	}
	fmt.Print("確認のためもう一度入力してください:")
	fmt.Println()
	reEnterPassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	passwordMatch = HashPasswordCheck(reEnterPassword, hash)
	if !passwordMatch {
		return nil, fmt.Errorf("入力が一致しませんでした\n")
	}
	return hash, nil
}
func PasswordHash(password []byte) ([]byte, error) {
	resBytes, err := bcrypt.GenerateFromPassword(password, 10)
	return resBytes, err
}
func HashPasswordCheck(password, hash []byte) bool {
	err := bcrypt.CompareHashAndPassword(hash, password)
	return err == nil
}
