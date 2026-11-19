package store

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DemoModel is the model priced and served by the default demo channels.
const DemoModel = "gpt-4o-mini"

// SeedDefaults creates two demo channels pointing at the mock upstream and a
// price for DemoModel, only when no channel exists yet.
func SeedDefaults(db *gorm.DB, mockURL string) error {
	var n int64
	if err := db.Model(&Channel{}).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	chans := []Channel{
		{Name: "OpenAI (mock)", Type: ChannelOpenAI, BaseURL: mockURL + "/openai", APIKey: "mock-openai-key",
			Models: datatypes.JSONSlice[string]{DemoModel}, ModelMap: datatypes.NewJSONType(map[string]string{}),
			Priority: 1, Enabled: true},
		{Name: "Anthropic (mock)", Type: ChannelAnthropic, BaseURL: mockURL + "/anthropic", APIKey: "mock-anthropic-key",
			Models: datatypes.JSONSlice[string]{DemoModel}, ModelMap: datatypes.NewJSONType(map[string]string{DemoModel: "claude-haiku-4-5"}),
			Priority: 2, Enabled: true},
	}
	if err := db.Create(&chans).Error; err != nil {
		return err
	}
	return db.Where(ModelPrice{Model: DemoModel}).
		FirstOrCreate(&ModelPrice{Model: DemoModel, InputPerM: 150_000, OutputPerM: 600_000}).Error
}
