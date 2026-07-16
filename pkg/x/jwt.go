package x

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/kataras/iris/v12"
	"strings"
	"time"
)

type Claims struct {
	UserId   string
	Username string
	Roles    []string
	Name     string
	jwt.StandardClaims
}

type JWTClaims struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type JwtUser struct {
	Id       string
	Username string
	Name     string
	Roles    []string
}

type JwtUserClaims struct {
	UserId   string
	Username string
	Name     string
	Roles    []string
	jwt.StandardClaims
}

func GetToken(ctx iris.Context, tokenKey string) string {
	header := tokenKey
	token := ctx.GetHeader(header)
	if token == "" {
		token = ctx.GetHeader(strings.ToLower(header))
	}
	if token == "" {
		token = ctx.URLParamTrim(header)
	}
	if token == "" {
		token = ctx.URLParamTrim(strings.ToLower(header))
	}
	return token
}

func GenerateJwtToken(signKey string, jwtUser *JwtUser, expireTime time.Time) (*JWTClaims, error) {
	issuer := "cias.cn"
	claims := &JwtUserClaims{
		UserId:   jwtUser.Id,
		Username: jwtUser.Username,
		Roles:    jwtUser.Roles,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireTime.Unix(),
			Issuer:    issuer,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(signKey))
	jwtClaims := JWTClaims{
		AccessToken: token,
		TokenType:   "jwt",
		ExpiresIn:   expireTime.Unix(),
	}
	return &jwtClaims, err
}

// GenerateToken 根据用户的用户名和密码产生token
func GenerateToken(signKey string, userId string, username, password string) (string, error) {
	//设置token有效时间
	nowTime := time.Now()
	expireTime := nowTime.Add(3 * time.Hour)
	claims := Claims{
		UserId:   userId,
		Username: username,
		StandardClaims: jwt.StandardClaims{
			// 过期时间
			ExpiresAt: expireTime.Unix(),
			// 指定token发行人
			Issuer: "cias.cn",
		},
	}

	tokenClaims := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//该方法内部生成签名字符串，再用于获取完整、已签名的token
	token, err := tokenClaims.SignedString(signKey) //tokenClaims.SignedString(jwtSecret)
	return token, err
}

// ParseToken 根据传入的token值获取到Claims对象信息，（进而获取其中的用户名和密码）
func ParseToken(signKey, token string) (*Claims, error) {
	//用于解析鉴权的声明，方法内部主要是具体的解码和校验的过程，最终返回*Token
	tokenClaims, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(signKey), nil //jwtSecret, nil
	})
	if tokenClaims != nil {
		// 从tokenClaims中获取到Claims对象，并使用断言，将该对象转换为我们自己定义的Claims
		// 要传入指针，项目中结构体都是用指针传递，节省空间。
		if claims, ok := tokenClaims.Claims.(*Claims); ok && tokenClaims.Valid {
			return claims, nil
		}
	}
	return nil, err
}
