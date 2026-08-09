package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/desktop/internal/localagent"
)

const (
	appearanceFileName     = "appearance.json"
	appearanceSchema       = "v1"
	appearanceCommandEvent = "zeitboard:appearance-command"
)

var civilClockPattern = regexp.MustCompile(`^$|^(?:[01]\d|2[0-3]):[0-5]\d$`)

type LocalNightRuleDTO struct {
	Enabled            bool    `json:"enabled"`
	Preset             string  `json:"preset"`
	LeadHours          float64 `json:"leadHours"`
	FallbackStartLocal string  `json:"fallbackStartLocal"`
	FallbackEndLocal   string  `json:"fallbackEndLocal"`
}

type LocalAppearanceStateDTO struct {
	Theme              string            `json:"theme"`
	ReducedStimulation bool              `json:"reducedStimulation"`
	NightRule          LocalNightRuleDTO `json:"nightRule"`
}

type LocalAppearanceSaveInput struct {
	State        LocalAppearanceStateDTO `json:"state"`
	BaseRevision uint64                  `json:"baseRevision"`
}

type LocalAppearanceEnvelopeDTO struct {
	State    LocalAppearanceStateDTO `json:"state"`
	Revision uint64                  `json:"revision"`
	Conflict bool                    `json:"conflict"`
}

type agentNightRuleInput struct {
	Enabled            bool    `json:"enabled"`
	Preset             string  `json:"preset"`
	LeadHours          float64 `json:"lead_hours"`
	FallbackStartLocal string  `json:"fallback_start_local"`
	FallbackEndLocal   string  `json:"fallback_end_local"`
}

type agentAppearancePatch struct {
	Theme              *string              `json:"theme,omitempty"`
	ReducedStimulation *bool                `json:"reduced_stimulation,omitempty"`
	NightRule          *agentNightRuleInput `json:"night_rule,omitempty"`
}

type agentNightRuleProjection struct {
	Enabled            bool    `json:"enabled"`
	Preset             string  `json:"preset"`
	LeadHours          float64 `json:"lead_hours"`
	FallbackStartLocal string  `json:"fallback_start_local"`
	FallbackEndLocal   string  `json:"fallback_end_local"`
}

type agentAppearanceProjection struct {
	SchemaVersion      string                   `json:"schema_version"`
	Theme              string                   `json:"theme"`
	ReducedStimulation bool                     `json:"reduced_stimulation"`
	NightRule          agentNightRuleProjection `json:"night_rule"`
	Scope              string                   `json:"scope"`
}

func defaultLocalAppearanceState() LocalAppearanceStateDTO {
	return LocalAppearanceStateDTO{
		Theme: "auto",
		NightRule: LocalNightRuleDTO{
			Preset:    "amber",
			LeadHours: 2,
		},
	}
}

func validateLocalAppearanceState(state LocalAppearanceStateDTO) error {
	switch state.Theme {
	case "auto", "light", "dark", "black", "amber", "contrast":
	default:
		return errors.New("theme must be auto, light, dark, black, amber, or contrast")
	}
	switch state.NightRule.Preset {
	case "amber", "black", "dark":
	default:
		return errors.New("night preset must be amber, black, or dark")
	}
	if math.IsNaN(state.NightRule.LeadHours) || math.IsInf(state.NightRule.LeadHours, 0) || state.NightRule.LeadHours < 0 || state.NightRule.LeadHours > 12 {
		return errors.New("night lead hours must be from 0 through 12")
	}
	if !civilClockPattern.MatchString(state.NightRule.FallbackStartLocal) || !civilClockPattern.MatchString(state.NightRule.FallbackEndLocal) {
		return errors.New("night fallback times must be empty or HH:MM civil times")
	}
	return nil
}

func (a *App) LoadLocalAppearanceState(local LocalAppearanceStateDTO) (LocalAppearanceEnvelopeDTO, error) {
	if err := validateLocalAppearanceState(local); err != nil {
		return LocalAppearanceEnvelopeDTO{}, err
	}
	a.appearanceMu.Lock()
	defer a.appearanceMu.Unlock()
	if a.appearancePersisted {
		return a.appearanceEnvelopeLocked(false), nil
	}
	if err := a.persistAppearanceLocked(local, 1); err != nil {
		return a.appearanceEnvelopeLocked(false), errors.New("appearance settings could not be stored")
	}
	a.appearance = local
	a.appearanceRevision = 1
	a.appearancePersisted = true
	a.appearanceErr = ""
	return a.appearanceEnvelopeLocked(false), nil
}

func (a *App) SaveLocalAppearanceState(input LocalAppearanceSaveInput) (LocalAppearanceEnvelopeDTO, error) {
	if err := validateLocalAppearanceState(input.State); err != nil {
		return LocalAppearanceEnvelopeDTO{}, err
	}
	a.appearanceMu.Lock()
	defer a.appearanceMu.Unlock()
	if input.BaseRevision != a.appearanceRevision {
		return a.appearanceEnvelopeLocked(true), nil
	}
	nextRevision := a.appearanceRevision + 1
	if err := a.persistAppearanceLocked(input.State, nextRevision); err != nil {
		a.appearanceErr = "Appearance settings could not be stored."
		return a.appearanceEnvelopeLocked(false), errors.New("appearance settings could not be stored")
	}
	a.appearance = input.State
	a.appearanceRevision = nextRevision
	a.appearancePersisted = true
	a.appearanceErr = ""
	return a.appearanceEnvelopeLocked(false), nil
}

func (a *App) appearanceEnvelopeLocked(conflict bool) LocalAppearanceEnvelopeDTO {
	return LocalAppearanceEnvelopeDTO{State: a.appearance, Revision: a.appearanceRevision, Conflict: conflict}
}

func (a *App) currentAppearance() LocalAppearanceStateDTO {
	a.appearanceMu.RLock()
	defer a.appearanceMu.RUnlock()
	return a.appearance
}

func (a *App) loadAppearanceFromDisk() error {
	if a.configDir == "" {
		return nil
	}
	path := filepath.Join(a.configDir, appearanceFileName)
	stored, revision, found, recovered, err := readAppearanceFile(path)
	if err != nil || !found {
		return err
	}
	if recovered {
		if err := appearanceStore(path).restorePrimary(stored, revision); err != nil {
			return fmt.Errorf("restore appearance settings backup: %w", err)
		}
	}
	a.appearanceMu.Lock()
	a.appearance = stored
	a.appearanceRevision = revision
	a.appearancePersisted = true
	a.appearanceErr = ""
	a.appearanceMu.Unlock()
	return nil
}

func (a *App) persistAppearanceLocked(state LocalAppearanceStateDTO, revision uint64) error {
	if a.configDir == "" {
		return nil
	}
	return appearanceStore(filepath.Join(a.configDir, appearanceFileName)).write(state, revision)
}

func (a *App) applyAppearanceTool(arguments json.RawMessage) (agentAppearanceProjection, error) {
	var patch agentAppearancePatch
	if err := decodeStrictJSON(arguments, &patch); err != nil {
		return agentAppearanceProjection{}, localagent.UserError("Appearance arguments are invalid. No display setting was changed.")
	}
	if patch.Theme == nil && patch.ReducedStimulation == nil && patch.NightRule == nil {
		return agentAppearanceProjection{}, localagent.UserError("At least one appearance setting is required. No display setting was changed.")
	}

	a.appearanceMu.Lock()
	state := a.appearance
	if patch.Theme != nil {
		state.Theme = *patch.Theme
	}
	if patch.ReducedStimulation != nil {
		state.ReducedStimulation = *patch.ReducedStimulation
	}
	if patch.NightRule != nil {
		state.NightRule = LocalNightRuleDTO{
			Enabled:            patch.NightRule.Enabled,
			Preset:             patch.NightRule.Preset,
			LeadHours:          patch.NightRule.LeadHours,
			FallbackStartLocal: patch.NightRule.FallbackStartLocal,
			FallbackEndLocal:   patch.NightRule.FallbackEndLocal,
		}
	}
	if err := validateLocalAppearanceState(state); err != nil {
		a.appearanceMu.Unlock()
		return agentAppearanceProjection{}, localagent.UserError(err.Error() + ". No display setting was changed.")
	}
	nextRevision := a.appearanceRevision + 1
	if err := a.persistAppearanceLocked(state, nextRevision); err != nil {
		a.appearanceErr = "Appearance settings could not be stored."
		a.appearanceMu.Unlock()
		return agentAppearanceProjection{}, errors.New("persist appearance state")
	}
	a.appearance = state
	a.appearanceRevision = nextRevision
	a.appearancePersisted = true
	a.appearanceErr = ""
	envelope := a.appearanceEnvelopeLocked(false)
	a.appearanceMu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, appearanceCommandEvent, envelope)
	}
	return projectAgentAppearance(state), nil
}

func projectAgentAppearance(state LocalAppearanceStateDTO) agentAppearanceProjection {
	return agentAppearanceProjection{
		SchemaVersion:      appearanceSchema,
		Theme:              state.Theme,
		ReducedStimulation: state.ReducedStimulation,
		NightRule: agentNightRuleProjection{
			Enabled:            state.NightRule.Enabled,
			Preset:             state.NightRule.Preset,
			LeadHours:          state.NightRule.LeadHours,
			FallbackStartLocal: state.NightRule.FallbackStartLocal,
			FallbackEndLocal:   state.NightRule.FallbackEndLocal,
		},
		Scope: "local reversible display state",
	}
}

// appearanceStore is the durable settings file for display preferences. The
// read/write/restore machinery is shared with every other settings file; only
// the schema name and what counts as a valid state differ.
func appearanceStore(path string) localSettingsStore[LocalAppearanceStateDTO] {
	return localSettingsStore[LocalAppearanceStateDTO]{
		Path:     path,
		Schema:   appearanceSchema,
		Name:     "appearance",
		Validate: validateLocalAppearanceState,
	}
}

func readAppearanceFile(path string) (LocalAppearanceStateDTO, uint64, bool, bool, error) {
	return appearanceStore(path).read()
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}
