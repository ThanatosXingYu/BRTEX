package agent

import (
	"regexp"
	"strings"
)

// The main agent's run is one-shot: a turn that ends with text and no tool call
// ends the whole turn, and progress then depends on the operator typing
// "继续". The shape that actually happens in production is not "no tools at
// all" but "the model announced its next step and then stopped": the closing
// text promises work ("先看内容。", "Let me pull the facts.", "…并确认节拍：")
// that was never performed.
//
// The detector below reads only the model's own closing sentence, so a normal
// answer or summary is never touched: a reply that concludes, answers, asks
// the operator a question, gives advice or waits for a worker has no pending
// step in its last sentence and is left alone.

// actionStrength grades how explicit the "announced but not acted" reading is.
type actionStrength int

const (
	// weakAction: the sentence frames a next step ("先看 Order 模型的这两个 scope。").
	weakAction actionStrength = iota + 1
	// strongAction: the sentence itself promises more content
	// ("…并确认节拍：", "Let me pull the anchor facts I'll cite.").
	strongAction
)

var (
	// "Let me pull the facts." / "I'll now check the log."
	englishActionRE = regexp.MustCompile(`(?i)\b(let me|let'?s|now i|next,? i|i'?ll now|i will now|i'?m going to)\b`)
	// The model is waiting for a worker, the planner or the operator before it
	// can move on — the turn legitimately ends here.
	awaitingRE = regexp.MustCompile(`(等|待|一旦)[^，。；：、]{0,15}(worker|planner|规划者|意图|结果|完成|跑完|跑出|通知|回复|回话|你|您)`)
	// The next step belongs to somebody else: the planner drives the workers.
	delegatedRE = regexp.MustCompile(`(?i)(planner|worker|规划者)[^，。；：、]{0,12}(接下来|下一步|该|负责|去|会)`)
	// Advice to the operator is not a promise to act.
	adviceRE = regexp.MustCompile(`建议|不妨|你可以|您可以|可以试试|推荐|供你参考|如果你愿意|如果您愿意|如果你想|如果你需要|由你决定|你决定`)
	// A finished statement is a conclusion, not an announcement.
	doneRE = regexp.MustCompile(`已(经)?(完成|验证|确认|提交|输出|生成|通过|结束|停止|写好|写完|改好|修好|部署|启动|跑完|做完|处理|检查|测试|分析|复现|定位|枚举|查找|读取|查看|抓取|执行|调用|核对|整理|汇总|解决|修复)|完成了|搞定了|已就绪`)
	// An imperative to oneself ("看内容。", "确认一下：").
	bareActionRE = regexp.MustCompile(`^(看|查看|查|查清|读|读取|打开|列出|找|抓|抓取|跑|运行|执行|测试|复现|定位|枚举|检索|汇总|整理|梳理|检查|分析|修|修改|提交|启动|拉|取|获取|下载|上传|打印|核对|试|调|拆|删|建|写|处理|确认一下)`)
	// Sentence / line breaks. A period only ends a sentence when a space or the
	// end of the text follows it, so "3.8.4" stays intact.
	sentenceSplitRE = regexp.MustCompile(`[。！？!?；;]+|\.(?:\s+|$)`)
	// "…，并看验签顺序" — the closing sentence adds one more promised action.
	coordinationRE = regexp.MustCompile(`，\s*并`)
)

// activeVerbs are verbs that mean "work to do". They only carry meaning next
// to a step marker, so a summary that merely mentions them is not flagged.
var activeVerbs = []string{
	"查看", "看", "查", "读", "找", "打开", "列出", "抓", "执行", "调用", "翻", "扫",
	"定位", "枚举", "检索", "汇总", "整理", "复现", "梳理", "写", "改", "统计",
	"比对", "对比", "下载", "上传", "获取", "清点", "测试", "验证", "分析", "检查",
	"提交", "启动", "修", "部署", "打印", "核对", "拉", "建", "做", "处理",
	"跑", "试", "拆", "删",
}

// pendingAction reports whether the closing text of a turn announces work the
// model said it would do but never did, and how explicit that announcement is.
func pendingAction(text string) (actionStrength, bool) {
	seg := finalSegment(text)
	if seg == "" {
		return 0, false
	}
	// The model asked the operator something: the human has to answer, so the
	// turn waits here on purpose.
	if strings.HasSuffix(seg, "？") || strings.HasSuffix(seg, "?") {
		return 0, false
	}
	if adviceRE.MatchString(seg) || awaitingRE.MatchString(seg) || delegatedRE.MatchString(seg) {
		return 0, false
	}
	// "…并确认节拍：" — the sentence promises content that never arrives.
	if strings.HasSuffix(seg, "：") || strings.HasSuffix(seg, ":") {
		return strongAction, true
	}
	// "现在在 abc 上复现这个 oracle，并看验签顺序" — the sentence ends with an
	// extra promised action, so the turn cannot be complete.
	if hasCoordinatedAction(seg) {
		return strongAction, true
	}
	if doneRE.MatchString(seg) {
		return 0, false
	}
	if englishActionRE.MatchString(seg) {
		return strongAction, true
	}
	if hasPlanMarker(seg) && hasActiveVerb(seg) {
		return weakAction, true
	}
	if bareActionRE.MatchString(seg) && len([]rune(seg)) <= 24 {
		return strongAction, true
	}
	return 0, false
}

// finalSegment returns the last sentence of the last non-empty line of text —
// the part of a reply that carries the model's closing move.
func finalSegment(text string) string {
	lines := strings.Split(text, "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			last = s
			break
		}
	}
	if last == "" {
		return ""
	}
	parts := sentenceSplitRE.Split(last, -1)
	for i := len(parts) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(parts[i]); s != "" {
			return s
		}
	}
	return ""
}

// hasPlanMarker reports whether the sentence frames the next step as something
// the model is about to do ("先看…", "接下来我来…", "下面先找到…").
func hasPlanMarker(s string) bool {
	for _, m := range []string{
		"接下来", "下一步", "然后", "让我", "我来", "我先", "即将", "打算", "逐一", "下面",
		"现在先", "现在就", "现在去", "现在要", "现在需", "现在我", "现在开", "现在启",
		"现在做", "现在把", "现在写", "现在跑", "现在查", "现在看", "现在读", "现在试",
		"现在调", "现在抓", "现在拿", "现在复", "现在验", "现在测", "现在检", "现在定",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return hasStepMarker(s)
}

// hasStepMarker matches the standalone step marker 先, but not the words it is
// part of (首先 / 优先 / 先前).
func hasStepMarker(s string) bool {
	runes := []rune(s)
	for i, r := range runes {
		if r != '先' {
			continue
		}
		if i > 0 {
			switch runes[i-1] {
			case '首', '优', '最':
				continue
			}
		}
		if i+1 < len(runes) && runes[i+1] == '前' {
			continue
		}
		return true
	}
	return false
}

// hasCoordinatedAction reports whether the sentence appends another promised
// action ("…，并看验签顺序"). The verb has to sit right behind the conjunction,
// otherwise the sentence only mentions the work in passing.
func hasCoordinatedAction(s string) bool {
	for _, loc := range coordinationRE.FindAllStringIndex(s, -1) {
		tail := []rune(s[loc[1]:])
		if len(tail) > 12 {
			tail = tail[:12]
		}
		if hasActiveVerb(string(tail)) {
			return true
		}
	}
	return false
}

// hasActiveVerb reports whether the sentence names work to be done.
func hasActiveVerb(s string) bool {
	runes := []rune(s)
	for _, v := range activeVerbs {
		vr := []rune(v)
		for i := 0; i+len(vr) <= len(runes); i++ {
			if string(runes[i:i+len(vr)]) != v {
				continue
			}
			// "跑着的 worker"、"查到的表"、"提交了 3 条" describe a state, not a
			// promise to do something.
			if i+len(vr) < len(runes) {
				switch runes[i+len(vr)] {
				case '着', '的', '中', '了':
					continue
				}
			}
			return true
		}
	}
	return false
}
