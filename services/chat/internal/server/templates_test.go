package server_test

import (
	"chatflow/services/chat/internal/server"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChatUsesEmbeddedTemplate(t *testing.T) {
	s, err := server.New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := s.GetRouter()
	response := httptest.NewRecorder()
	err = router.HTMLRender.Instance("users.html", gin.H{
		"Me": "Template test", "MyID": 1, "OnlineIDs": []int{},
	}).Render(response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Body.String(), "Template test") || !strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatal("chat page did not render")
	}
}
