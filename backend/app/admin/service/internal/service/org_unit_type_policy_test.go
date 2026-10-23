package service

import (
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	identityV1 "go-wind-admin/api/gen/go/identity/service/v1"
)

func TestNormalizeOrgSubtypePath(t *testing.T) {
	cases := []struct {
		name    string
		orgType identityV1.OrgUnit_Type
		path    []string
		want    []string
		wantErr bool
	}{
		{name: "门店业态", orgType: identityV1.OrgUnit_STORE, path: []string{"SHOPPING_MALL"}, want: []string{"SHOPPING_MALL"}},
		{name: "门店缺细分", orgType: identityV1.OrgUnit_STORE, path: nil, wantErr: true},
		{name: "部门职能转大写", orgType: identityV1.OrgUnit_DEPARTMENT, path: []string{" leasing "}, want: []string{"LEASING"}},
		{name: "部门非法职能", orgType: identityV1.OrgUnit_DEPARTMENT, path: []string{"NO_SUCH_CODE"}, wantErr: true},
		{name: "项目缺细分", orgType: identityV1.OrgUnit_PROJECT, path: nil, wantErr: true},
		{name: "事业部可空", orgType: identityV1.OrgUnit_DIVISION, path: nil, want: nil},
		{name: "公司不支持细分", orgType: identityV1.OrgUnit_COMPANY, path: []string{"SHOPPING_MALL"}, wantErr: true},
		{name: "集团可空", orgType: identityV1.OrgUnit_GROUP, path: nil, want: nil},
		{name: "层级过深", orgType: identityV1.OrgUnit_DEPARTMENT, path: []string{"LEASING", "A", "B", "C"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeOrgSubtypePath(tc.orgType, tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got = %v, want = %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got = %v, want = %v", got, tc.want)
				}
			}
		})
	}
}

func TestUpdateMaskIncludes(t *testing.T) {
	if !updateMaskIncludes(nil, "subtype_path") {
		t.Fatal("nil mask should include every field")
	}
	if !updateMaskIncludes(&fieldmaskpb.FieldMask{Paths: []string{"subtypePath"}}, "subtype_path") {
		t.Fatal("camelCase path should match snake_case field")
	}
	if updateMaskIncludes(&fieldmaskpb.FieldMask{Paths: []string{"name"}}, "subtype_path") {
		t.Fatal("unrelated path should not match")
	}
}
