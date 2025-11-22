package auth

import (
	"github.com/golang-jwt/jwt/v5"

	"github.com/thepenn/devsys/model"
)

type AuthResponse struct {
	Token    string   `json:"token"`
	User     UserInfo `json:"user"`
	Redirect string   `json:"redirect,omitempty"`
}

type UserInfo struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar_url"`
	ForgeID  int64  `json:"forge_id"`
	Admin    bool   `json:"admin"`
	Provider string `json:"provider"`
}

type SessionClaims struct {
	UserID int64  `json:"uid"`
	Login  string `json:"login"`
	jwt.RegisteredClaims
}

func toUserInfo(user *model.User, provider string) UserInfo {
	return UserInfo{
		ID:       user.ID,
		Login:    user.Login,
		Email:    user.Email,
		Avatar:   user.Avatar,
		ForgeID:  user.ForgeID,
		Admin:    user.Admin,
		Provider: provider,
	}
}
