package db

import (
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/providers"
	"9router/proxy/internal/thinking"
)

func applyComboStrategies(s *SettingsData, raw map[string]any) {
	csMap, ok := raw["comboStrategies"].(map[string]any)
	if !ok {
		return
	}
	s.ComboStrategies = make(map[string]ComboStrategy, len(csMap))
	for k, v := range csMap {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		strat := handlerutil.GetString(vm, "strategy")
		if strat == "" {
			strat = handlerutil.GetString(vm, "fallbackStrategy")
		}
		sticky := 0
		if sl, ok := vm["stickyLimit"].(float64); ok && sl > 0 {
			sticky = int(sl)
		} else if sl, ok := vm["stickyRoundRobinLimit"].(float64); ok && sl > 0 {
			sticky = int(sl)
		}
		entry := ComboStrategy{
			Strategy:    strat,
			StickyLimit: sticky,
			JudgeModel:  handlerutil.GetString(vm, "judgeModel"),
		}
		if timeout, ok := vm["firstTokenTimeoutMs"].(float64); ok && timeout > 0 && timeout <= float64((1<<63-1)/1000000) && timeout == float64(int64(timeout)) {
			entry.FirstTokenTimeoutMs = int(timeout)
		}
		// Unlike the timeout, zero is meaningful here ("re-include at once"),
		// so only an absent or invalid value falls back to the default.
		if grace, ok := vm["firstTokenGraceMs"].(float64); ok && grace >= 0 && grace <= float64((1<<63-1)/1000000) && grace == float64(int64(grace)) {
			g := int(grace)
			entry.FirstTokenGraceMs = &g
		}
		entry.SmartPolicy = handlerutil.GetString(vm, "smartPolicy")
		if ss, ok := vm["sessionSticky"].(bool); ok {
			entry.SessionSticky = &ss
		}
		if ct, ok := vm["confidenceThreshold"].(float64); ok && ct > 0 {
			entry.ConfidenceThreshold = &ct
		}
		entry.RouterModel = handlerutil.GetString(vm, "routerModel")
		// An absent flag reads as false, which is the behaviour every
		// combo had before the flag existed: rotation stays on unless
		// the operator turned it off for this combo.
		if rso, ok := vm["respectStoredOrder"].(bool); ok {
			entry.RespectStoredOrder = rso
		}
		s.ComboStrategies[k] = entry
	}
}

// applyProviderStrategies reads the per-provider strategy map.
//
// The two rotations are carried separately: the provider card saves pool
// rotation to `rotateStrategy` and connection rotation to `fallbackStrategy`,
// and folding them into one field made saving either silently arm the other.
// `proxyPoolId` pins a single pool and outranks both.
// `stickyRoundRobinLimit` / `stickyLimit` belong to connection rotation.
func applyProviderStrategies(s *SettingsData, raw map[string]any) {
	ps, ok := raw["providerStrategies"].(map[string]any)
	if !ok {
		return
	}
	s.ProviderStrategies = make(map[string]ProviderStrategy, len(ps))
	for k, v := range ps {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rotateStrat := handlerutil.GetString(vm, "rotateStrategy")
		if rotateStrat == "" {
			rotateStrat = handlerutil.GetString(vm, "fallbackStrategy")
		}
		sticky := 0
		if sl, ok := vm["stickyLimit"].(float64); ok && sl > 0 {
			sticky = int(sl)
		} else if sl, ok := vm["stickyRoundRobinLimit"].(float64); ok && sl > 0 {
			sticky = int(sl)
		}
		strat := ProviderStrategy{
			ProxyPoolID:         handlerutil.GetString(vm, "proxyPoolId"),
			RotateStrategy:      rotateStrat,
			StickyLimit:         sticky,
			ProxyRotateStrategy: handlerutil.GetString(vm, "rotateStrategy"),
			ConnRotateStrategy:  handlerutil.GetString(vm, "fallbackStrategy"),
		}
		if sma, ok := vm["strictModelAssignment"].(bool); ok {
			strat.StrictModelAssignment = sma
		}
		s.ProviderStrategies[k] = strat
	}
}

// applyProviderThinking reads the per-provider reasoning overrides.
//
// Only a mode inside the allowed vocabulary is kept: the mode reaches
// reasoning_effort on the wire, and a provider rejects an unknown effort.
func applyProviderThinking(s *SettingsData, raw map[string]any) {
	pt, ok := raw["providerThinking"].(map[string]any)
	if !ok {
		return
	}
	s.ProviderThinking = make(map[string]ProviderThinking, len(pt))
	for k, v := range pt {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if mode, valid := thinking.NormalizeProviderThinkingMode(handlerutil.GetString(vm, "mode")); valid {
			s.ProviderThinking[k] = ProviderThinking{Mode: mode}
		}
	}
}

func applyCapacityAdapters(s *SettingsData, raw map[string]any) {
	caRaw, ok := raw["capacityAdapter"].(map[string]any)
	if !ok {
		return
	}
	s.CapacityAdapter = make(map[string]CapacityAdapterEntry, len(caRaw))
	for k, v := range caRaw {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		enabled := true
		if en, ok := vm["enabled"].(bool); ok {
			enabled = en
		}
		rr := false
		if r, ok := vm["roundRobin"].(bool); ok {
			rr = r
		}
		var models []string
		if rawModels, ok := vm["models"].([]any); ok {
			for _, rm := range rawModels {
				if ms, ok := rm.(string); ok && ms != "" {
					models = append(models, providers.CanonicalModelName(ms))
				}
			}
		}
		s.CapacityAdapter[k] = CapacityAdapterEntry{
			Enabled:    enabled,
			RoundRobin: rr,
			Models:     models,
		}
	}
}

// applyProviderOverrides reads the per-provider header overrides.
//
// The settings blob can arrive from a backup import, so a wrong shape is
// dropped rather than trusted.
func applyProviderOverrides(s *SettingsData, raw map[string]any) {
	po, ok := raw["providerOverrides"].(map[string]any)
	if !ok {
		return
	}
	s.ProviderOverrides = make(map[string]ProviderOverrides, len(po))
	for k, v := range po {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		s.ProviderOverrides[k] = ProviderOverrides{Headers: stringMap(entry["headers"])}
	}
}

// applyModelParameters parses per-model and per-combo parameter overrides.
func applyModelParameters(s *SettingsData, raw map[string]any) {
	mpRaw, ok := raw["modelParameters"].(map[string]any)
	if !ok {
		return
	}
	s.ModelParameters = make(map[string]ModelParameters, len(mpRaw))
	for target, val := range mpRaw {
		vm, ok := val.(map[string]any)
		if !ok {
			continue
		}
		var p ModelParameters

		// temperature
		if t, ok := vm["temperature"].(float64); ok {
			p.Temperature = &t
		}

		// top_p / topP
		if tp, ok := vm["topP"].(float64); ok {
			p.TopP = &tp
		} else if tp, ok := vm["top_p"].(float64); ok {
			p.TopP = &tp
		}

		// top_k / topK
		if tk, ok := vm["topK"].(float64); ok {
			itk := int(tk)
			p.TopK = &itk
		} else if tk, ok := vm["top_k"].(float64); ok {
			itk := int(tk)
			p.TopK = &itk
		}

		// repetition_penalty / repetitionPenalty
		if rp, ok := vm["repetitionPenalty"].(float64); ok {
			p.RepetitionPenalty = &rp
		} else if rp, ok := vm["repetition_penalty"].(float64); ok {
			p.RepetitionPenalty = &rp
		}

		// frequency_penalty / frequencyPenalty
		if fp, ok := vm["frequencyPenalty"].(float64); ok {
			p.FrequencyPenalty = &fp
		} else if fp, ok := vm["frequency_penalty"].(float64); ok {
			p.FrequencyPenalty = &fp
		}

		// presence_penalty / presencePenalty
		if pp, ok := vm["presencePenalty"].(float64); ok {
			p.PresencePenalty = &pp
		} else if pp, ok := vm["presence_penalty"].(float64); ok {
			p.PresencePenalty = &pp
		}

		// max_tokens / maxTokens
		if mt, ok := vm["maxTokens"].(float64); ok && mt > 0 {
			imt := int(mt)
			p.MaxTokens = &imt
		} else if mt, ok := vm["max_tokens"].(float64); ok && mt > 0 {
			imt := int(mt)
			p.MaxTokens = &imt
		}

		// context_length / contextLength
		if cl, ok := vm["contextLength"].(float64); ok && cl > 0 {
			icl := int(cl)
			p.ContextLength = &icl
		} else if cl, ok := vm["context_length"].(float64); ok && cl > 0 {
			icl := int(cl)
			p.ContextLength = &icl
		}

		// stream
		if st := handlerutil.GetString(vm, "stream"); st != "" {
			p.Stream = &st
		}

		s.ModelParameters[target] = p
	}
}
