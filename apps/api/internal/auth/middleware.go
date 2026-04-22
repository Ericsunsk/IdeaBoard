package auth

import (
	"api/internal/platform/config"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"sync"

	"api/internal/platform/redis"
	"api/internal/platform/response"
	"api/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtSecret     []byte
	jwtSecretOnce sync.Once
)

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func JWTSigningSecret() []byte {
	jwtSecretOnce.Do(func() {
		cfg := config.Load()
		if secret := strings.TrimSpace(cfg.JWT.Secret); secret != "" {
			jwtSecret = []byte(secret)
			return
		}
		if config.IsProductionEnv() {
			log.Fatal("[Auth] JWT_SECRET 未配置；生产环境必须显式设置固定密钥")
		}

		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			log.Fatalf("failed to initialize JWT secret: %v", err)
		}

		jwtSecret = []byte(base64.StdEncoding.EncodeToString(buf))
		log.Printf("[Auth] JWT_SECRET 未配置，当前运行在非生产环境；已生成一次性开发密钥，服务重启后旧 token 会失效")
	})

	return jwtSecret
}

// AuthMiddleware 认证中间件
func AuthMiddleware(rdb *redis.Client, userRepo *users.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, http.StatusUnauthorized, "未提供认证令牌")
			c.Abort()
			return
		}

		// Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Error(c, http.StatusUnauthorized, "令牌格式错误")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return JWTSigningSecret(), nil
		})

		if err != nil || !token.Valid {
			response.Error(c, http.StatusUnauthorized, "令牌无效或已过期")
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			response.Error(c, http.StatusUnauthorized, "令牌解析失败")
			c.Abort()
			return
		}

		// 检查Redis中是否还有这个token
		tokenKey := TokenRedisKey(claims.UserID)
		storedToken, err := rdb.Get(c.Request.Context(), tokenKey)
		if err != nil || storedToken != tokenString {
			response.Error(c, http.StatusUnauthorized, "令牌已失效")
			c.Abort()
			return
		}

		if userRepo != nil {
			user, err := userRepo.GetByID(claims.UserID)
			if err != nil || user.Status != 1 {
				response.Error(c, http.StatusUnauthorized, "用户不存在或已停用")
				c.Abort()
				return
			}

			c.Set("user_id", user.ID)
			c.Set("username", user.Username)
			c.Set("role", user.Role)
			c.Next()
			return
		}

		// 将用户信息存入context
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// AdminMiddleware 管理员中间件 - 通过角色判断是否为管理员
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			response.Error(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
