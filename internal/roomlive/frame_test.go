package roomlive

import (
	"encoding/json"
	"testing"
)

func TestATurnFrameCarriesTheTurnAsJSON(t *testing.T) {
	data := Frame{V: Version, Type: FrameTurn, Room: "r", Seq: 3, ID: "t1", Turn: json.RawMessage(`{"ID":"t1","Kind":"turn"}`)}.encode()
	var got struct {
		Type string
		Turn struct{ ID, Kind string }
		HTML *string `json:"html"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != FrameTurn || got.Turn.ID != "t1" || got.Turn.Kind != "turn" || got.HTML != nil {
		t.Errorf("the frame is %s", data)
	}
}

func TestATurnThatIsNotJSONIsSentAsARefresh(t *testing.T) {
	data := Frame{V: Version, Type: FrameTurn, Room: "r", Thread: "1", Seq: 4, ID: "t1", Turn: json.RawMessage(`{not json`)}.encode()
	var got Frame
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != FrameRefresh || got.Seq != 4 || got.Thread != "1" || got.Turn != nil {
		t.Errorf("the frame is %s", data)
	}
}
