package store

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the user's preferences, mirroring the keys the app's
// settings pane edits.
type Config struct {
	// Layout.
	LeftPanelShow        bool `json:"left_panel_show"`
	LeftPanelWidth       int  `json:"left_panel_width"`
	RightPanelShow       bool `json:"right_panel_show"`
	RightPanelWidth      int  `json:"right_panel_width"`
	UseSystemWindowFrame bool `json:"use_system_window_frame"`

	// Behaviour.
	WriteMode            string `json:"write_mode"`
	HistoryLimit         int    `json:"history_limit"`
	HistoryEnabled       bool   `json:"history_enabled"`
	Locale               string `json:"locale,omitempty"`
	Theme                string `json:"theme"`
	ChoiceMode           int    `json:"choice_mode"`
	ShowTitleOnTray      bool   `json:"show_title_on_tray"`
	LaunchAtLogin        bool   `json:"launch_at_login"`
	HideAtLaunch         bool   `json:"hide_at_launch"`
	SendUsageData        bool   `json:"send_usage_data"`
	CmdAfterHostsApply   string `json:"cmd_after_hosts_apply"`
	RemoveDuplicate      bool   `json:"remove_duplicate_records"`
	HideDockIcon         bool   `json:"hide_dock_icon"`
	MultiChooseFolderAll bool   `json:"multi_chose_folder_switch_all"`
	TrayMiniWindow       bool   `json:"tray_mini_window"`
	LightweightMode      bool   `json:"lightweight_mode"`
	QuitOnClose          bool   `json:"quit_on_close"`

	// Find window.
	FindIsRegexp           bool  `json:"find_is_regexp"`
	FindIsIgnoreCase       bool  `json:"find_is_ignore_case"`
	FindResultColumnWidths []int `json:"find_result_column_widths"`

	// Proxy.
	UseProxy      bool   `json:"use_proxy"`
	ProxyProtocol string `json:"proxy_protocol"`
	ProxyHost     string `json:"proxy_host"`
	ProxyPort     int    `json:"proxy_port"`

	// Domain resolution.
	DNSProvider  string `json:"dns_provider"`
	DNSCustomURL string `json:"dns_custom_url"`

	// Remote hosts.
	RefreshRemoteOnStartup bool `json:"refresh_remote_hosts_on_startup"`

	// Local HTTP API.
	HTTPAPIOn        bool `json:"http_api_on"`
	HTTPAPIOnlyLocal bool `json:"http_api_only_local"`
	HTTPAPIPort      int  `json:"http_api_port"`

	// Updates.
	AutoCheckUpdate bool `json:"auto_check_update"`
}

// DefaultHTTPAPIPort is the port the HTTP API listens on unless set.
const DefaultHTTPAPIPort = 50761

// DefaultConfig returns the preferences of a fresh install.
func DefaultConfig() *Config {
	return &Config{
		LeftPanelShow:          true,
		LeftPanelWidth:         270,
		RightPanelWidth:        240,
		WriteMode:              "append",
		HistoryLimit:           50,
		HistoryEnabled:         true,
		Theme:                  "system",
		ChoiceMode:             2,
		TrayMiniWindow:         true,
		ProxyProtocol:          "http",
		DNSProvider:            "alidns",
		HTTPAPIOnlyLocal:       true,
		HTTPAPIPort:            DefaultHTTPAPIPort,
		AutoCheckUpdate:        true,
		FindResultColumnWidths: []int{},
	}
}

// LoadConfig reads internal/config.json, falling back to the defaults.
func LoadConfig(p Paths) *Config {
	data, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		return DefaultConfig()
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig()
	}
	cfg.normalize()
	return cfg
}

func (c *Config) normalize() {
	if c.HTTPAPIPort < 1 || c.HTTPAPIPort > 65535 {
		c.HTTPAPIPort = DefaultHTTPAPIPort
	}
	switch c.Theme {
	case "light", "dark", "system":
	default:
		c.Theme = "system"
	}
	switch c.ProxyProtocol {
	case "http", "https", "socks5":
	default:
		c.ProxyProtocol = "http"
	}
	if c.ProxyPort > 65535 {
		c.ProxyPort = 65535
	}
	if len(c.FindResultColumnWidths) != 0 && len(c.FindResultColumnWidths) != 3 {
		c.FindResultColumnWidths = []int{}
	}
	if c.LeftPanelWidth < 100 {
		c.LeftPanelWidth = 100
	}
	if c.RightPanelWidth < 100 {
		c.RightPanelWidth = 100
	}
}

// Save writes internal/config.json.
func (c *Config) Save(p Paths) error {
	body, err := marshalIndentJSON(c)
	if err != nil {
		return err
	}
	return AtomicWrite(p.ConfigFile, body)
}

// Allow is the list of preference keys the settings pane updates, and how
// each is applied.
func (c *Config) ApplyPatch(patch map[string]any) error {
	for key, value := range patch {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		switch key {
		case "left_panel_show":
			err = json.Unmarshal(raw, &c.LeftPanelShow)
		case "left_panel_width":
			err = json.Unmarshal(raw, &c.LeftPanelWidth)
		case "right_panel_show":
			err = json.Unmarshal(raw, &c.RightPanelShow)
		case "right_panel_width":
			err = json.Unmarshal(raw, &c.RightPanelWidth)
		case "use_system_window_frame":
			err = json.Unmarshal(raw, &c.UseSystemWindowFrame)
		case "write_mode":
			err = json.Unmarshal(raw, &c.WriteMode)
		case "history_limit":
			err = json.Unmarshal(raw, &c.HistoryLimit)
		case "history_enabled":
			err = json.Unmarshal(raw, &c.HistoryEnabled)
		case "locale":
			err = json.Unmarshal(raw, &c.Locale)
		case "theme":
			err = json.Unmarshal(raw, &c.Theme)
		case "choice_mode":
			err = json.Unmarshal(raw, &c.ChoiceMode)
		case "show_title_on_tray":
			err = json.Unmarshal(raw, &c.ShowTitleOnTray)
		case "launch_at_login":
			err = json.Unmarshal(raw, &c.LaunchAtLogin)
		case "hide_at_launch":
			err = json.Unmarshal(raw, &c.HideAtLaunch)
		case "send_usage_data":
			err = json.Unmarshal(raw, &c.SendUsageData)
		case "cmd_after_hosts_apply":
			err = json.Unmarshal(raw, &c.CmdAfterHostsApply)
		case "remove_duplicate_records":
			err = json.Unmarshal(raw, &c.RemoveDuplicate)
		case "hide_dock_icon":
			err = json.Unmarshal(raw, &c.HideDockIcon)
		case "multi_chose_folder_switch_all":
			err = json.Unmarshal(raw, &c.MultiChooseFolderAll)
		case "tray_mini_window":
			err = json.Unmarshal(raw, &c.TrayMiniWindow)
		case "lightweight_mode":
			err = json.Unmarshal(raw, &c.LightweightMode)
		case "quit_on_close":
			err = json.Unmarshal(raw, &c.QuitOnClose)
		case "find_is_regexp":
			err = json.Unmarshal(raw, &c.FindIsRegexp)
		case "find_is_ignore_case":
			err = json.Unmarshal(raw, &c.FindIsIgnoreCase)
		case "find_result_column_widths":
			err = json.Unmarshal(raw, &c.FindResultColumnWidths)
		case "use_proxy":
			err = json.Unmarshal(raw, &c.UseProxy)
		case "proxy_protocol":
			err = json.Unmarshal(raw, &c.ProxyProtocol)
		case "proxy_host":
			err = json.Unmarshal(raw, &c.ProxyHost)
		case "proxy_port":
			err = json.Unmarshal(raw, &c.ProxyPort)
		case "dns_provider":
			err = json.Unmarshal(raw, &c.DNSProvider)
		case "dns_custom_url":
			err = json.Unmarshal(raw, &c.DNSCustomURL)
		case "refresh_remote_hosts_on_startup":
			err = json.Unmarshal(raw, &c.RefreshRemoteOnStartup)
		case "http_api_on":
			err = json.Unmarshal(raw, &c.HTTPAPIOn)
		case "http_api_only_local":
			err = json.Unmarshal(raw, &c.HTTPAPIOnlyLocal)
		case "http_api_port":
			err = json.Unmarshal(raw, &c.HTTPAPIPort)
		case "auto_check_update":
			err = json.Unmarshal(raw, &c.AutoCheckUpdate)
		default:
			return fmt.Errorf("unknown config key %q", key)
		}
		if err != nil {
			return fmt.Errorf("config %s: %w", key, err)
		}
	}
	c.normalize()
	return nil
}

// Flat returns the preferences as a plain object, the shape the app reads.
func (c *Config) Flat() map[string]any {
	body, _ := json.Marshal(c)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return out
}
