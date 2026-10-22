package ontology

import (
	"testing"

	"go-wind-admin/app/admin/service/internal/data/ent"
)

// 02 期渲染分支单测（02 号文档 §4.4）：Manchester 渲染 + 对称成对去重 + 链保序。

func TestRenderObjectPropertyAxioms(t *testing.T) {
	names := map[uint32]string{1: "P", 2: "Q", 3: "R"}
	cases := []struct {
		name string
		ax   *ent.OntAxiom
		want string
	}{
		{"子属性", axiomRow(1, AxiomSubObjectPropertyOf, 2, 1, "", "", "", 0), "SubPropertyOf(Q P)"},
		{"定义域", axiomRow(2, AxiomObjectPropertyDomain, 1, 5, "", "", "", 0), "Domain(P 5)"},
		{"值域", axiomRow(3, AxiomObjectPropertyRange, 1, 5, "", "", "", 0), "Range(P 5)"},
		{"特征", axiomRow(4, AxiomObjectPropertyCharacteristic, 1, 0, PropertyCharacteristicIRIs[2], "", "", 0), "Characteristic(P, transitive)"},
		{"逆属性", axiomRow(5, AxiomInverseOf, 1, 2, "", "", "", 0), "InverseOf(P Q)"},
		{"等价", axiomRow(6, AxiomEquivalentObjectProperties, 1, 2, "", "", "", 0), "EquivalentProperties(P Q)"},
		{"不相交", axiomRow(7, AxiomDisjointObjectProperties, 1, 2, "", "", "", 0), "DisjointProperties(P Q)"},
		{
			"属性链",
			axiomRow(8, AxiomSubPropertyChainOf, 1, 0, "", EncodeChainMemberIDs([]uint32{2, 3}), "", 0),
			"SubPropertyChainOf(Q ∘ R P)",
		},
	}
	for _, tc := range cases {
		if got := RenderAxiom(tc.ax, names); got != tc.want {
			t.Fatalf("%s 渲染不符：got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestRenderSymmetricPairDedup(t *testing.T) {
	names := map[uint32]string{1: "P", 2: "Q"}
	// 成对镜像行：(P,Q) 渲染，(Q,P) 抑制为空——两条只出一条
	forward := axiomRow(1, AxiomInverseOf, 1, 2, "", "", "", 0)
	backward := axiomRow(2, AxiomInverseOf, 2, 1, "", "", "", 0)
	if got := RenderAxiom(forward, names); got != "InverseOf(P Q)" {
		t.Fatalf("正向往应渲染：%q", got)
	}
	if got := RenderAxiom(backward, names); got != "" {
		t.Fatalf("镜像行应抑制为空：%q", got)
	}
}

func TestRenderChainInvalidFallback(t *testing.T) {
	names := map[uint32]string{1: "P"}
	ax := axiomRow(1, AxiomSubPropertyChainOf, 1, 0, "", "{bad", "", 0)
	if got := RenderAxiom(ax, names); got != "SubPropertyChainOf(<invalid-chain> ∘ P)" {
		t.Fatalf("非法链应回退占位：%q", got)
	}
}
