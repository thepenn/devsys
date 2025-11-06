package model

import (
	"errors"
	"regexp"
)

var reUsername = regexp.MustCompile("^[a-zA-Z0-9-_.]+$")

var errUserLoginInvalid = errors.New("invalid user login")

const maxLoginLen = 250

type User struct {
	ID            int64         `json:"id"             gorm:"column:id;primaryKey;autoIncrement"`
	ForgeID       int64         `json:"forge_id,omitempty" gorm:"column:forge_id;uniqueIndex:uq_users_forge_remote_id;uniqueIndex:uq_users_forge_login"`
	ForgeRemoteID ForgeRemoteID `json:"forge_remote_id"    gorm:"column:forge_remote_id;size:191;uniqueIndex:uq_users_forge_remote_id"`
	Login         string        `json:"login"          gorm:"column:login;size:191;uniqueIndex:uq_users_forge_login"`
	AccessToken   string        `json:"-"              gorm:"column:access_token;type:text"`
	RefreshToken  string        `json:"-"              gorm:"column:refresh_token;type:text"`
	Expiry        int64         `json:"-"              gorm:"column:expiry"`
	Email         string        `json:"email"          gorm:"column:email;size:500"`
	Avatar        string        `json:"avatar_url"     gorm:"column:avatar;size:500"`
	Admin         bool          `json:"admin,omitempty" gorm:"column:admin"`
	Hash          string        `json:"-"              gorm:"column:hash;size:191;uniqueIndex"`
	OrgID         int64         `json:"org_id"         gorm:"column:org_id"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) Validate() error {
	switch {
	case len(u.Login) == 0:
		return errUserLoginInvalid
	case len(u.Login) > maxLoginLen:
		return errUserLoginInvalid
	case !reUsername.MatchString(u.Login):
		return errUserLoginInvalid
	default:
		return nil
	}
}
