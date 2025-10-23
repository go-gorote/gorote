package gorote

import (
	"crypto/rsa"
	"fmt"
	"mime/multipart"
	"reflect"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/golang-jwt/jwt/v5"
)

type HandlerJWTProtected func(jwt.Claims) *fiber.Error

type parseInfo struct {
	hasQuery  bool
	hasJSON   bool
	hasParams bool
	hasForm   bool
}

func Check() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).JSON(map[string]string{"status": "OK"})
	}
}

func IsWsMiddleware() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(ctx) {
			return fiber.NewError(fiber.StatusUpgradeRequired, "upgrade required")
		}
		return ctx.Next()
	}
}

func JWTProtected(claims jwt.Claims, jwtSecret string, handles ...HandlerJWTProtected) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if err := ValidateOrGetJWT(claims, GetAccessToken(ctx), jwtSecret); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}
		for _, handle := range handles {
			if err := handle(claims); err != nil {
				return err
			}
		}
		ctx.Locals("claimsData", claims)
		return ctx.Next()
	}
}

func JWTProtectedRSA(claims jwt.Claims, publicKey *rsa.PublicKey, handles ...HandlerJWTProtected) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if err := ValidateOrGetJWTRSA(claims, GetAccessToken(ctx), publicKey); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}
		for _, handle := range handles {
			if err := handle(claims); err != nil {
				return err
			}
		}
		ctx.Locals("claimsData", claims)
		return ctx.Next()
	}
}

func ValidationMiddleware(requestStruct any) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		typ := reflect.TypeOf(requestStruct)
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}

		if typ.Kind() != reflect.Struct {
			return fiber.NewError(fiber.StatusInternalServerError, "validation target must be a struct")
		}

		info := analyzeStructTags(typ)
		instance := reflect.New(typ).Interface()
		if info.hasParams {
			if err := ctx.ParamsParser(instance); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("invalid params: %v", err))
			}
		}
		if info.hasQuery {
			if err := ctx.QueryParser(instance); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("invalid query: %v", err))
			}
		}
		if info.hasJSON {
			if err := ctx.BodyParser(instance); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
			}
		}

		if info.hasForm {
			parseFormFields(ctx, instance)
		}

		if err := ValidateStruct(instance); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		ctx.Locals("validatedData", instance)
		return ctx.Next()
	}
}

func analyzeStructTags(typ reflect.Type) *parseInfo {
	info := &parseInfo{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if _, ok := field.Tag.Lookup("query"); ok {
			info.hasQuery = true
		}
		if _, ok := field.Tag.Lookup("json"); ok {
			info.hasJSON = true
		}
		if _, ok := field.Tag.Lookup("param"); ok {
			info.hasParams = true
		}
		if _, ok := field.Tag.Lookup("form"); ok {
			info.hasForm = true
		}
	}
	return info
}

func parseFormFields(ctx *fiber.Ctx, instance any) {
	val := reflect.ValueOf(instance).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		formTag := field.Tag.Get("form")
		if formTag == "" {
			continue
		}

		switch field.Type {
		case reflect.TypeOf((*multipart.FileHeader)(nil)):
			file, err := ctx.FormFile(formTag)
			if err == nil && fieldVal.CanSet() {
				fieldVal.Set(reflect.ValueOf(file))
			}
		case reflect.TypeOf([]*multipart.FileHeader{}):
			form, err := ctx.MultipartForm()
			if err == nil && form != nil && form.File[formTag] != nil {
				fieldVal.Set(reflect.ValueOf(form.File[formTag]))
			}
		default:
			if fieldVal.Kind() == reflect.String && fieldVal.CanSet() {
				fieldVal.SetString(ctx.FormValue(formTag))
			}
		}
	}
}

func Cached(ttl time.Duration) func(ctx *fiber.Ctx) error {
	config := cache.Config{
		Expiration: ttl,
	}
	return cache.New(config)
}

func Limited(max int) func(c *fiber.Ctx) error {
	config := limiter.Config{
		Max: max,
		LimitReached: func(c *fiber.Ctx) error {
			return fiber.ErrTooManyRequests
		},
	}
	return limiter.New(config)
}
