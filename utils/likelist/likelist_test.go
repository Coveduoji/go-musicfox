package likelist

import "testing"

func TestConfirmedLikeStateCanBeToggled(t *testing.T) {
	const songID = -9511
	defer SetLikeSong(songID, false)
	SetLikeSong(songID, false)
	for _, want := range []bool{true, false, true, false} {
		// A second click must observe the last successful server operation immediately.
		target := !IsLikeSong(songID)
		if target != want {
			t.Fatal("next click does not reverse the previous action")
		}
		SetLikeSong(songID, target)
		if IsLikeSong(songID) != want {
			t.Fatal("confirmed result was not reflected locally")
		}
	}
}
