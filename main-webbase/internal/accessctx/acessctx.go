package accessctx

import (
	"context"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"main-webbase/internal/models"
)

type MembershipSummary struct {
	NodeID  bson.ObjectID
	OrgPath string
	PosKey  string
}

type ViewerAccess struct {
	Memberships    []MembershipSummary
	SubtreePaths   []string        // รวม path ทั้งตัวเองและลูก
	SubtreeNodeIDs []bson.ObjectID // Node IDs ที่เข้าถึงได้ (ไว้ match role_visibility)
}

// -------------------------
// Public API
// -------------------------

// BuildViewerAccess สร้างภาพรวมการเข้าถึงของผู้ใช้ปัจจุบัน
func BuildViewerAccess(ctx context.Context, db *mongo.Database, userID bson.ObjectID) (*ViewerAccess, error) {
	mCol := db.Collection("memberships")
	nodeCol := db.Collection("org_units")

	// 2) memberships ที่ active ของ user
	cur, err := mCol.Find(ctx, bson.M{"user_id": userID, "active": true})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ms []models.Membership
	if err := cur.All(ctx, &ms); err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		// ไม่มี role เลย → subtree ว่าง (เห็นได้เฉพาะ public ตาม policy ของ feed เวลา query)
		return &ViewerAccess{
			Memberships:    nil,
			SubtreePaths:   nil,
			SubtreeNodeIDs: nil,
		}, nil
	}

	// 3) หา node_id ของแต่ละ org_path ที่ user เป็นสมาชิก
	//    และคำนวณ subtree: nodes ที่ path == org_path หรือ ancestors มี org_path
	//    ทำเป็น union จากทุก membership
	pathSet := make(map[string]struct{})
	nodeIDSet := make(map[bson.ObjectID]struct{})
	var summaries []MembershipSummary

	for _, m := range ms {
		// 3.1 หา node ของ path นี้เพื่อเก็บ node_id ใน summary
		var node models.OrgUnitNode
		if err := nodeCol.FindOne(ctx, bson.M{"org_path": m.OrgPath, "status": "active"}).Decode(&node); err == nil {
			summaries = append(summaries, MembershipSummary{
				NodeID:  node.ID,
				OrgPath: m.OrgPath,
				PosKey:  m.PositionKey,
			})
		} else {
			// ถ้าไม่พบ node, ก็ยังใส่ summary ไว้ด้วย path/posKey
			summaries = append(summaries, MembershipSummary{
				OrgPath: m.OrgPath,
				PosKey:  m.PositionKey,
			})
		}

		// 3.2 ดึงซับทรีสำหรับ path นี้
		//     filter: { $or: [ {path: m.OrgPath}, {ancestors: m.OrgPath} ] }
		subCur, err := nodeCol.Find(ctx, bson.M{
			"status": "active",
			"$or": bson.A{
				bson.M{"org_path": m.OrgPath},
				bson.M{"ancestors": m.OrgPath},
			},
		})
		if err != nil {
			return nil, err
		}
		var subNodes []models.OrgUnitNode
		if err := subCur.All(ctx, &subNodes); err != nil {
			return nil, err
		}
		for _, n := range subNodes {
			pathSet[n.OrgPath] = struct{}{}
			nodeIDSet[n.ID] = struct{}{}
		}
	}

	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	nodeIDs := make([]bson.ObjectID, 0, len(nodeIDSet))
	for id := range nodeIDSet {
		nodeIDs = append(nodeIDs, id)
	}
	// ทำให้อ่านง่าย / deterministic
	slices.Sort(paths)

	return &ViewerAccess{
		Memberships:    summaries,
		SubtreePaths:   paths,
		SubtreeNodeIDs: nodeIDs,
	}, nil
}

// VisibilityMatch สร้างเงื่อนไขกรองโพสต์:
// - public: คือโพสต์ที่ "ไม่มี" role_visibility
// - private: role_visibility ∈ viewer.SubtreeNodeIDs
//
// includePublic = true → รวม public ด้วย
func (v *ViewerAccess) VisibilityMatch(includePublic bool) bson.M {
	private := bson.M{"role_visibility": bson.M{"$in": v.SubtreeNodeIDs}}
	if includePublic {
		return bson.M{
			"$or": bson.A{
				bson.M{"role_visibility": bson.M{"$exists": false}}, // public
				private,
			},
		}
	}
	return private
}
