package config

import "testing"

// The rule an ending ride is judged by is the rule the product states unless a process is told
// otherwise, and a process is told otherwise in only one place: the profile that installs a
// demonstration.
func TestFinishLandingIsEnforcedUnlessTheDemonstrationRelaxesIt(t *testing.T) {
	for _, read := range []struct {
		name    string
		setting string
		profile string
		want    string
		refused bool
	}{
		{name: "unset in a deployment", profile: ProductionEnvironment, want: EnforcedFinishLanding},
		{name: "unset in the demonstration", profile: DemoEnvironment, want: EnforcedFinishLanding},
		{
			name: "enforced in a deployment", setting: EnforcedFinishLanding,
			profile: ProductionEnvironment, want: EnforcedFinishLanding,
		},
		{
			name: "enforced in the demonstration", setting: EnforcedFinishLanding,
			profile: DemoEnvironment, want: EnforcedFinishLanding,
		},
		{
			name: "relaxed in the demonstration", setting: TestRideLifecycleFinishLanding,
			profile: DemoEnvironment, want: TestRideLifecycleFinishLanding,
		},
		{
			name: "relaxed in a deployment", setting: TestRideLifecycleFinishLanding,
			profile: ProductionEnvironment, refused: true,
		},
		{name: "a value nobody declares", setting: "whenever", profile: DemoEnvironment, refused: true},
	} {
		t.Run(read.name, func(t *testing.T) {
			t.Setenv(FinishLandingVariable, read.setting)
			landing, err := loadFinishLanding(read.profile)
			if read.refused {
				if err == nil {
					t.Fatalf("the setting %q was accepted in the %s profile", read.setting, read.profile)
				}
				return
			}
			if err != nil {
				t.Fatalf("the setting %q was refused in the %s profile: %v",
					read.setting, read.profile, err)
			}
			if landing != read.want {
				t.Fatalf("the setting %q is read as %q, want %q", read.setting, landing, read.want)
			}
		})
	}
}

// Every value this build declares names the profile that admits it, and exactly one of them relaxes a
// rule: a second relaxed value would be a second way of saying the same thing.
func TestTheDeclaredLandingValuesAreTheOnesTheLoaderAdmits(t *testing.T) {
	relaxed := 0
	for _, declared := range finishLandingValues {
		if declared.name == "" {
			t.Fatal("a declared landing value names nothing")
		}
		if declared.admittedProfile != "" {
			relaxed++
			if declared.admittedProfile != DemoEnvironment {
				t.Errorf("the value %q is admitted in the %s profile",
					declared.name, declared.admittedProfile)
			}
		}
	}
	if relaxed != 1 {
		t.Errorf("%d landing values are admitted in one profile, want one", relaxed)
	}
	if DefaultFinishLanding != EnforcedFinishLanding {
		t.Errorf("the default landing rule is %q, want %q", DefaultFinishLanding, EnforcedFinishLanding)
	}
}
