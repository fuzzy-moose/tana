package httpapi

import "testing"

func TestInvalidPathIDs(t *testing.T) {
	h := testHandler(t)
	for _, id := range []string{"invalid", "0", "-1", "1.5", "9223372036854775808"} {
		for _, endpoint := range []struct{ method, path, body string }{
			{"GET", "/api/libraries/" + id, ""},
			{"PATCH", "/api/libraries/" + id, `{"name":"Renamed"}`},
			{"DELETE", "/api/libraries/" + id, ""},
			{"POST", "/api/libraries/" + id + "/availability-check", ""},
			{"GET", "/api/galleries/" + id, ""},
			{"GET", "/api/galleries/" + id + "/pages/1/image", ""},
		} {
			request(t, h, endpoint.method, endpoint.path, endpoint.body, 404)
		}
	}
}
