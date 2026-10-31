
import sys

# 1. Update settings.go
path = r"C:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\db\settings.go"
with open(path, "r", encoding="utf-8") as f:
    content = f.read()

target1 = '\tSystemPromptSuffix      string          `json:"systemPromptSuffix,omitempty"`'
insert1 = target1 + '\n\tChatRewriteEnabled      bool            `json:"chatRewriteEnabled"`\n\tChatRewriteSystem       string          `json:"chatRewriteSystem,omitempty"`\n\tChatRewriteAssistant    string          `json:"chatRewriteAssistant,omitempty"`\n\tChatRewriteTemperature  float64         `json:"chatRewriteTemperature,omitempty"`\n\tChatRewriteTopP         float64         `json:"chatRewriteTopP,omitempty"`'

target2 = '\t\tSystemPromptSuffix:      "",'
insert2 = target2 + '\n\t\tChatRewriteEnabled:      true,\n\t\tChatRewriteTemperature:  0.9,\n\t\tChatRewriteTopP:         0.95,'

if "ChatRewriteEnabled" not in content:
    content = content.replace(target1, insert1)
    content = content.replace(target2, insert2)
    with open(path, "w", encoding="utf-8", newline="") as f:
        f.write(content)
    print("Updated settings.go")
else:
    print("settings.go already updated")

# 2. Update settings_parse.go
path_parse = r"C:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\db\settings_parse.go"
with open(path_parse, "r", encoding="utf-8") as f:
    parse_content = f.read()

target_parse1 = "\tapplyPromptSettings(s, raw)"
insert_parse1 = target_parse1 + "\n\tapplyRewriteSettings(s, raw)"

func_rewrite = """
func applyRewriteSettings(s *SettingsData, raw map[string]any) {
	if v, ok := raw["chatRewriteEnabled"].(bool); ok {
		s.ChatRewriteEnabled = v
	}
	if v := handlerutil.GetString(raw, "chatRewriteSystem"); v != "" {
		s.ChatRewriteSystem = v
	}
	if v := handlerutil.GetString(raw, "chatRewriteAssistant"); v != "" {
		s.ChatRewriteAssistant = v
	}
	if v, ok := raw["chatRewriteTemperature"].(float64); ok && v >= 0 {
		s.ChatRewriteTemperature = v
	}
	if v, ok := raw["chatRewriteTopP"].(float64); ok && v >= 0 {
		s.ChatRewriteTopP = v
	}
}
"""

if "applyRewriteSettings" not in parse_content:
    parse_content = parse_content.replace(target_parse1, insert_parse1)
    parse_content += func_rewrite
    with open(path_parse, "w", encoding="utf-8", newline="") as f:
        f.write(parse_content)
    print("Updated settings_parse.go")
else:
    print("settings_parse.go already updated")
