package main

import "testing"

func TestActionRouting(t *testing.T) {
	cases := []struct {
		kind, name string
		want       RequestAction
	}{
		{"PROCESS", `C:\\Program Files\\Demo\\player.exe`, ActionCloseProcess},
		{"DRIVER", `Realtek High Definition Audio (HDAUDIO\\FUNC_01&VEN_10EC)`, ActionInspectAudio},
		{"DRIVER", `USB Audio Device`, ActionInspectAudio},
		{"DRIVER", `蓝牙音频设备`, ActionInspectAudio},
		{"DRIVER", `\\FileSystem\\srvnet`, ActionInspectDriver},
		{"SERVICE", `SomeService`, ActionInspectService},
		{"UNKNOWN", `whatever`, ActionNone},
		{"", ``, ActionNone},
	}
	for _, c := range cases {
		got := classifyAction(PowerRequest{Kind: c.kind, Name: c.name})
		if got != c.want {
			t.Fatalf("%q / %q: got %q want %q", c.kind, c.name, got, c.want)
		}
		if c.want != ActionNone && actionTitle(got) == "" {
			t.Fatal("action has no title")
		}
		if actionDescription(got) == "" {
			t.Fatal("action has no description")
		}
	}
}

func TestDriverCannotRouteToTerminateProcess(t *testing.T) {
	for _, name := range []string{"Realtek High Definition Audio", `\\FileSystem\\srvnet`, `Unknown Driver`} {
		action := classifyAction(PowerRequest{Kind: "DRIVER", Name: name})
		if action == ActionCloseProcess {
			t.Fatalf("unsafe driver action: %s", name)
		}
	}
	if classifyAction(PowerRequest{Kind: "SERVICE", Name: "AudioSrv"}) == ActionCloseProcess {
		t.Fatal("service routed to terminate process")
	}
}

func TestAudioDriverScreenshot(t *testing.T) {
	r := ParseRequests("SYSTEM:\n[DRIVER] Realtek High Definition Audio (HDAUDIO\\FUNC_01&VEN_10EC&DEV_0287)\n")
	if len(r.Items) != 1 || r.Items[0].Section != "SYSTEM" || r.Items[0].Kind != "DRIVER" {
		t.Fatal(r.Items)
	}
	if classifyAction(r.Items[0]) != ActionInspectAudio {
		t.Fatal("Realtek driver not routed to audio diagnostics")
	}
}
