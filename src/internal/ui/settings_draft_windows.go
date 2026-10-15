//go:build windows

package ui

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
)

type settingsDraftState struct {
	Text   map[int]string
	Checks map[int]bool
	Combos map[int]int
	Editor string
}

func canonicalSettingsJSON(body string) string {
	var value interface{}
	if json.Unmarshal([]byte(body), &value) != nil {
		return body
	}
	b, _ := json.Marshal(value)
	return string(b)
}

func (f *form) settingsDraftState() settingsDraftState {
	s := settingsDraftState{Text: map[int]string{}, Checks: map[int]bool{}, Combos: map[int]int{}}
	for _, id := range []int{idOutputDir, idConfig, idTimeout, idOCREnginePath, idSevenZip, idOCRScore} {
		s.Text[id] = windowText(f.ctrls[id])
	}
	for _, id := range []int{idRecursive, idNoArchive, idNoOCR, idLock, idRemoteTasks, idAutoStart} {
		s.Checks[id] = f.isChecked(id)
	}
	for _, id := range []int{idTimeoutUnit, idLogLevel, idOCREngine} {
		s.Combos[id] = f.comboIndex(id)
	}
	body, _, _, _ := ConfigEditorBody(strings.TrimSpace(windowText(f.ctrls[idConfig])), filepath.Dir(f.dataLayout().Config()))
	if f.configOpen {
		body = windowText(f.ctrls[idCfgEditor])
	}
	s.Editor = canonicalSettingsJSON(body)
	return s
}

func (f *form) rememberSettingsDraft() {
	s := f.settingsDraftState()
	f.settingsBaseline = &s
}

func (f *form) settingsChanged() bool {
	return f.settingsBaseline != nil && !reflect.DeepEqual(*f.settingsBaseline, f.settingsDraftState())
}

func (f *form) discardSettingsDraft() {
	f.detectedOCREngine, f.detectedOCRPath = "", ""
	if f.settingsBaseline == nil {
		return
	}
	s := f.settingsBaseline
	for id, text := range s.Text {
		setWindowTextRaw(f.ctrls[id], text)
	}
	for id, checked := range s.Checks {
		var value uintptr
		if checked {
			value = bstChecked
		}
		send(f.ctrls[id], bmSetCheck, value, 0)
	}
	for id, index := range s.Combos {
		f.setComboSel(id, index)
	}
	setWindowTextRaw(f.ctrls[idCfgEditor], toCRLF(s.Editor))
	f.configOpen = false
	f.refreshCommand()
}

func (f *form) confirmLeaveSettings() bool {
	if !f.settingsChanged() {
		return true
	}
	answer := messageBox(f.hwnd, "设置尚未保存", "设置已修改，是否保存后离开？\r\n\r\n是：保存并离开\r\n否：放弃本次修改\r\n取消：留在设置页面", 3|mbIconQuestion)
	return settingsLeaveDecision(answer, func() bool {
		if f.configOpen && f.settingsBaseline != nil && canonicalSettingsJSON(windowText(f.ctrls[idCfgEditor])) != f.settingsBaseline.Editor {
			f.saveConfigEditor()
			if f.statusKind == statusKindFail {
				return false
			}
			f.loadSettingsFields()
		}
		if !f.saveSettings() {
			messageBox(f.hwnd, "设置保存失败", windowText(f.ctrls[idStatus]), mbOK|mbIconError)
			return false
		}
		return true
	}, f.discardSettingsDraft)
}
