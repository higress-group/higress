package tools

import (
	"testing"
)

func TestPermissionConfig(t *testing.T) {
	cfg, err := LoadPermissionConfig("../config/permission.yaml")
	if err != nil {
		t.Fatalf("加载权限配置失败: %v", err)
	}
	// 测试表和字段白名单
	err = cfg.CheckTableAndFields("user", []string{"id", "name"})
	if err != nil {
		t.Errorf("应允许user表的id和name字段: %v", err)
	}
	err = cfg.CheckTableAndFields("user", []string{"id", "xxx"})
	if err == nil {
		t.Error("不应允许user表的xxx字段")
	}
}

func TestIsSafeSQL(t *testing.T) {
	if !IsSafeSQL("select * from user") {
		t.Error("普通查询应为安全SQL")
	}
	if IsSafeSQL("drop table user;") {
		t.Error("drop语句应为危险SQL")
	}
	if IsSafeSQL("delete from user where id=1") {
		t.Error("delete语句应为危险SQL")
	}
} 