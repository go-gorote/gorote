package gorote

import (
	"crypto/rsa"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password")
	}
	return string(hashedPassword), nil
}

func CheckPasswordHash(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// ValidatePassword checks if a password has at least one uppercase letter and one symbol.
// It will return an error if the password does not meet these criteria.
// Example:
// password := "Senha@123"
// err := ValidatePassword(password)
//
//	if err != nil {
//		t.Errorf("erro ao validar senha: %v", err)
//	}
func ValidatePassword(password string) error {
	hasUpper := false
	hasSymbol := false
	for _, r := range password {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsSymbol(r) || unicode.IsPunct(r) {
			hasSymbol = true
		}
	}
	if !hasUpper {
		return fmt.Errorf("uppercase-password must contain at least one uppercase letter")
	}
	if !hasSymbol {
		return fmt.Errorf("symbol-password must contain at least one symbol")
	}
	return nil
}

// Pagination paginates the given data slice.
//
// It takes three parameters: page, limit and data.
// page is the number of the page to retrieve.
// limit is the number of elements per page.
// data is the slice to paginate.
//
// It returns an error if the page does not exist.
//
// It updates the data slice to contain only the elements of the requested page.
//
// Example:
//
// data := []int{1, 2, 3, 4, 5}
// err := Pagination(2, 2, &data)
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// fmt.Println(data) // [3, 4]
func Pagination[T any](page, limit uint, data *[]T) error {
	count := uint(len(*data))
	start := (page - 1) * limit
	end := page * limit
	if start >= count {
		return fmt.Errorf("page not exist")
	}
	if end > count {
		end = count
	}
	*data = (*data)[start:end]
	return nil
}

func GetAccessToken(ctx *fiber.Ctx) (token string) {
	token = ctx.Cookies("access_token")
	if token == "" {
		token = ctx.Get("Authorization")
	}
	return token
}

// RemoveInvisibleChars removes all invisible characters from a string.
// Invisible characters are defined as characters that are not printable
// and are not equal to the zero-width space character (U+200B).
// The function returns a new string containing only the visible characters.
//
// Example:
// input := "a\u200bb"
// output := RemoveInvisibleChars(input)
// fmt.Println(output) // "ab"
func RemoveInvisibleChars(input string) string {
	var result []rune
	for _, r := range input {
		if unicode.IsPrint(r) && r != '\u200b' {
			result = append(result, r)
		}
	}
	return string(result)
}

func getJSONFieldName(s any, field string) string {
	t := reflect.TypeOf(s)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if f, ok := t.FieldByName(field); ok {
		jsonTag := f.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			return splitJSONTag(jsonTag)
		}
	}
	return field
}

func splitJSONTag(tag string) string {
	for i, c := range tag {
		if c == ',' {
			return tag[:i]
		}
	}
	return tag
}

func validateStruct(data any) error {
	validate := validator.New()
	err := validate.Struct(data)
	if err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, err := range validationErrors {
				jsonField := getJSONFieldName(data, err.StructField())
				return fmt.Errorf("invalid validation: (field: '%s' is %s type: %s)", jsonField, err.ActualTag(), err.Type())
			}
		}
		return fmt.Errorf("invalid data: %s", err.Error())
	}
	return nil
}

// MustEnvAsTime returns a time.Duration based on the value of the environment variable
// at the given key. If the variable does not exist and defaultValue does exist, it panics.
// If the variable does exist, it attempts to convert it to an integer and returns a time.Duration
// with the given unit (seconds by default). If the conversion fails, it panics.
// The second argument is the value if the variable does not exist or has ""
//
// Example:
// var timeout = gorote.MustEnvAsTime("TIMEOUT", 30)
// // timeout is 30 seconds
func MustEnvAsTime(key string, defaultValue ...int) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		if len(defaultValue) > 0 {
			return time.Duration(defaultValue[0]) * time.Second
		}
		panic(fmt.Sprintf("variable %s is required", key))
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(fmt.Sprintf("failed to convert %s to integer: %v", key, err))
	}
	return time.Duration(value) * time.Second
}

// MustEnvAsInt returns an integer based on the value of the environment variable
// at the given key. If the variable does not exist and defaultValue does exist, it
// panics. If the variable does exist, it attempts to convert it to an integer
// and returns the value. If the conversion fails, it panics.
//
// Example:
// var timeout = gorote.MustEnvAsInt("TIMEOUT", 30)
// // timeout is 30
func MustEnvAsInt(key string, defaultValue ...int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		panic(fmt.Sprintf("variable %s is required", key))
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(fmt.Sprintf("failed to convert %s to integer: %v", key, err))
	}
	return value
}

// MustEnvAsString returns a string based on the value of the environment variable
// at the given key. If the variable does not exist and defaultValue does exist, it
// returns the defaultValue. If the variable does exist, it returns the value.
// If the variable does not exist and defaultValue does not exist, it panics.
//
// Example:
// var timeout = gorote.MustEnvAsString("TIMEOUT", "30")
// // timeout is "30"
func MustEnvAsString(key string, defaultValue ...string) string {
	value := os.Getenv(key)
	if value == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		panic(fmt.Sprintf("variable %s is required", key))
	}
	return value
}

// MustEnvPublicKeyRSA returns an RSA public key based on the value of the environment variable
// at the given key. If the variable does not exist, it panics.
//
// Example:
// var publicKey = gorote.MustEnvPublicKeyRSA("PUBLIC_KEY")
// // publicKey is the RSA public key from the PUBLIC_KEY environment variable
func MustEnvPublicKeyRSA(key string) *rsa.PublicKey {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("variable %s is required", key))
	}
	return MustReadPublicKeyFromString(value)
}

// MustEnvPrivateKeyRSA returns an RSA private key based on the value of the environment variable
// at the given key. If the variable does not exist, it panics.
//
// Example:
// var privateKey = gorote.MustEnvPrivateKeyRSA("PRIVATE_KEY")
// // privateKey is the RSA private key from the PRIVATE_KEY environment variable
func MustEnvPrivateKeyRSA(key string) *rsa.PrivateKey {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("variable %s is required", key))
	}
	return MustReadPrivateKeyFromString(value)
}
