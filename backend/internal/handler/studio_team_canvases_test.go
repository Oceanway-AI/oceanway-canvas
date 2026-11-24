package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioTeamCanvasTwoMembers(t *testing.T) {
	f:=newStudioFixture(t)
	var bobGeneration atomic.Int64
	bobGeneration.Store(2)
	f.workspace=func(w http.ResponseWriter,r *http.Request) {
		c,_:=r.Cookie("platform_session")
		uid,generation:=101,int64(1)
		if c!=nil && c.Value=="bob"{uid,generation=202,bobGeneration.Load()}
		if generation==0 || r.URL.Path!="/api/platform/v1/workspaces/team_alpha/context" {w.WriteHeader(403);return}
		json.NewEncoder(w).Encode(map[string]any{"success":true,"data":map[string]any{"id":"team_alpha","kind":"team","status":"active","team":map[string]any{"workspace_id":"team_alpha","owner_user_id":101,"expires_at":4000000000},"actor_user_id":uid,"actions":[]string{"workspace:read"},"membership_generation":generation}})
	}
	const base="/api/workspaces/team_alpha/canvas-projects"
	const alice="platform_session=alice"
	const bob="platform_session=bob"
	const id="/shared"
	const a="?membership_generation=1"
	const b="?membership_generation=2"
	create:=`{"mutationId":"shared-command","operations":[{"type":"create_canvas","id":"shared","patch":{"title":"Team canvas"}},{"type":"create_node","node":{"id":"n1","type":"text","metadata":{"content":"first"}}}]}`
	edit:=`{"mutationId":"shared-command","operations":[{"type":"patch_node","id":"n1","before":{"metadata.content":"first"},"patch":{"metadata.content":"second"}}]}`
	f.request(t,"PATCH",base+id+"/operations"+a,alice,create,200)
	f.request(t,"PATCH",base+id+"/operations"+b,bob,edit,200)
	f.request(t,"PATCH",base+id+"/operations"+b,bob,edit,200)
	stale:=strings.NewReplacer("shared-command","stale-edit","second","conflicting").Replace(edit)
	f.request(t,"PATCH",base+id+"/operations"+a,alice,stale,409)
	for _,pair:=range [][2]string{{a,alice},{b,bob}} {
		read:=f.request(t,"GET",base+id+pair[0],pair[1],"",200)
		if !strings.Contains(read.Body.String(),`"second"`){t.Fatal("stored edit missing")}
		list:=f.request(t,"GET",base+pair[0],pair[1],"",200)
		if !strings.Contains(list.Body.String(),"Team canvas"){t.Fatal("team canvas missing from list")}
		personal:=f.request(t,"GET","/api/canvas-projects",pair[1],"",200)
		if strings.Contains(personal.Body.String(),"Team canvas"){t.Fatal("team canvas in personal list")}
		f.request(t,"GET","/api/canvas-projects/shared",pair[1],"",404)
	}
	var project model.CanvasProject
	if err:=f.db.First(&project,"id=?","shared").Error;err!=nil || project.PlatformWorkspaceID!="team_alpha" || project.CreatorUserID!="101" || project.Revision!=2 {t.Fatalf("wrong persisted ownership/revision: %+v %v",project,err)}
	var receipt model.CanvasMutationReceipt
	if err:=f.db.First(&receipt,"canvas_id=? AND actor_user_id=?","shared","202").Error;err!=nil || receipt.MembershipGeneration!=2{t.Fatal("member attribution missing",err)}
	link:=f.request(t,"GET","/api/auth/workspace-context?canvas_id=shared&workspace_id=other",bob,"",200)
	if !strings.Contains(link.Body.String(),`"workspaceId":"team_alpha"`){t.Fatal("deep link lost original team")}
	bobGeneration.Store(0)
	f.request(t,"GET",base+id+b,bob,"",403)
	f.request(t,"PATCH",base+id+"/operations"+b,bob,edit,403)
	f.request(t,"GET","/api/auth/workspace-context?canvas_id=shared",bob,"",404)
	bobGeneration.Store(3)
	f.request(t,"GET",base+id+b,bob,"",409)
	f.request(t,"GET",base+id+"?membership_generation=3",bob,"",200)
}
