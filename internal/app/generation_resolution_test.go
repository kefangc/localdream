package app

import "testing"

func TestGenerationResolutionPrecisionAndBounds(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		mp          float64
	}{
		{"480p", "480p", 0.4}, {"720p", "720p", 0.9},
		{"0.1MP", "0.1MP", 0.1}, {"0.750MP", "0.75MP", 0.75},
		{"0.98MP", "0.98MP", 0.98}, {"1.0MP", "1MP", 1}, {"1.9MP", "1.9MP", 1.9},
	} {
		t.Run(tc.input, func(t *testing.T) {
			in := generationRequest{Resolution: tc.input}
			if err := normalizeGenerationResolution(&in); err != nil || in.Resolution != tc.want {
				t.Fatalf("normalize: %q, %v", in.Resolution, err)
			}
			_, mp := resolutionInputs("16:9", in.Resolution)
			if mp != tc.mp {
				t.Fatalf("compiler MP = %v, want %v", mp, tc.mp)
			}
		})
	}
	for _, invalid := range []string{"", "4K", "0.09MP", "1.91MP", "NaNMP", "+InfMP", "-0.5MP", "1e0MP", "0.5mp"} {
		if err := normalizeGenerationResolution(&generationRequest{Resolution: invalid}); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
}
