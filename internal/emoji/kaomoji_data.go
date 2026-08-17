package emoji

// kaomojiIcons is a hand-curated corpus — no standard "CLDR of kaomoji"
// exists, so this list draws from the common public kaomoji collections
// that most kaomoji pickers ship (grouped informally by mood below).
var kaomojiIcons = []Icon{
	// happy / cheerful
	{Glyph: "(^_^)", Name: "happy"},
	{Glyph: "(^o^)", Name: "cheerful"},
	{Glyph: "(*^v^*)", Name: "happy smile"},
	{Glyph: "(*^ω^*)", Name: "content"},
	{Glyph: "＼(^o^)／", Name: "hooray"},
	{Glyph: "(>v<)", Name: "very happy"},
	{Glyph: "(*>Ɐ<*)", Name: "excited happy"},
	{Glyph: "(^v^)", Name: "grin"},
	{Glyph: "('Ɐ｀)", Name: "warm smile"},
	{Glyph: "(*'v')", Name: "cheery"},
	{Glyph: "٩(◕‿◕)۶", Name: "delighted"},
	{Glyph: "(*>ω<*)", Name: "beaming"},
	{Glyph: "\\(^ω^)/", Name: "yay"},
	{Glyph: "(~v~)*", Name: "sparkling smile"},
	{Glyph: "ヽ(・Ɐ・)ﾉ", Name: "pleased"},

	// love / affection
	{Glyph: "(',,.ω.,,)ღ", Name: "love affection"},
	{Glyph: "(ღμ_μ)", Name: "in love"},
	{Glyph: "(' ω `ღ)", Name: "sweet love"},
	{Glyph: "(｡ღ‿ღ｡)", Name: "heart eyes"},
	{Glyph: "ღ(>͈ દ <͈ ༶ )", Name: "shy love"},
	{Glyph: "(*¯︶¯*)", Name: "adoring"},
	{Glyph: "('v`ʃღƪ)", Name: "loving"},
	{Glyph: "(◕‿◕)ღ", Name: "affectionate"},
	{Glyph: "❤(ӦｖӦ｡)", Name: "heart struck"},

	// laughing
	{Glyph: "(>v<)ﾉ", Name: "laughing wave"},
	{Glyph: "(*>m<*)", Name: "giggling"},
	{Glyph: "www", Name: "lol"},
	{Glyph: "(＾v＾)", Name: "laughing"},
	{Glyph: "(>ロ<)", Name: "cracking up"},
	{Glyph: "(*^艸^*)", Name: "snickering"},

	// blush / shy
	{Glyph: "(/ω\\)", Name: "shy"},
	{Glyph: "(/////)", Name: "blushing"},
	{Glyph: "(*/ω＼*)", Name: "bashful"},
	{Glyph: "(⁄ ⁄.⁄ω⁄.⁄ ⁄)", Name: "flustered blush"},
	{Glyph: "(*ﾉωﾉ)", Name: "embarrassed"},
	{Glyph: "(〃￣ω￣〃)", Name: "coy"},

	// sad / crying
	{Glyph: "('；ω；｀)", Name: "sad"},
	{Glyph: "(T﹏T)", Name: "crying"},
	{Glyph: "(;_;)", Name: "tearful"},
	{Glyph: "(T_T)", Name: "sobbing"},
	{Glyph: "(ノ_<)", Name: "whimpering"},
	{Glyph: "。゜('Ｔωｔ｀)゜。", Name: "bawling"},
	{Glyph: "(｡.︿.｡)", Name: "disappointed"},
	{Glyph: "(つ_<)", Name: "downcast"},
	{Glyph: "('；д；`)", Name: "distressed"},

	// angry / annoyed
	{Glyph: "(# ಠ益ಠ)", Name: "furious"},
	{Glyph: "٩(#ಠ益ಠ)۶", Name: "angry"},
	{Glyph: "(＃`Д')", Name: "mad"},
	{Glyph: "ヽ(`Д')ﾉ", Name: "annoyed"},
	{Glyph: "(-_-#)", Name: "irritated"},
	{Glyph: "(｀ε')", Name: "pouting"},
	{Glyph: "＼(-o-)／", Name: "exasperated"},
	{Glyph: "（＃'ω`）", Name: "grumpy"},

	// table flip / frustration
	{Glyph: "(ノo口o）ノ︵ ===", Name: "table flip"},
	{Glyph: "=== ノ( ゜-゜ノ)", Name: "put table back"},
	{Glyph: "(ノಠ益ಠ)ノ彡===", Name: "angry table flip"},
	{Glyph: "(ノ'口')ノ ︵ ===", Name: "flip in frustration"},

	// shrug / indifference
	{Glyph: "¯\\_(ツ)_/¯", Name: "shrug"},
	{Glyph: "\\('～｀)/", Name: "meh shrug"},
	{Glyph: "\\(￣ヘ￣)/", Name: "indifferent"},
	{Glyph: "(・_・)", Name: "unimpressed"},
	{Glyph: "( - ,_ゝ-)", Name: "whatever"},

	// surprised / shocked
	{Glyph: "(o_o)", Name: "surprised"},
	{Glyph: "(ooo)", Name: "shocked"},
	{Glyph: "Σ(o^o|||)", Name: "startled"},
	{Glyph: "(；ﾟДﾟ)", Name: "stunned"},
	{Glyph: "(ﾟｰﾟ)", Name: "wide eyed"},
	{Glyph: "( ゚д゚)", Name: "wtf"},
	{Glyph: "('^｀)", Name: "flustered surprise"},

	// confused
	{Glyph: "(・・？)", Name: "confused"},
	{Glyph: "(￣～￣;)", Name: "puzzled"},
	{Glyph: "(・Ɐ・)？", Name: "questioning"},
	{Glyph: "( ᴥ )？", Name: "huh"},
	{Glyph: "(oロo) !", Name: "baffled"},

	// thinking
	{Glyph: "(－‸ლ)", Name: "skeptical"},
	{Glyph: "(¬_¬)", Name: "side eye"},
	{Glyph: "(￢‿￢ )", Name: "smug"},
	{Glyph: "( -ω- )", Name: "pondering"},
	{Glyph: "(・口・;)", Name: "worried thinking"},

	// sleepy
	{Glyph: "(－ω－) zzZ", Name: "sleepy"},
	{Glyph: "(－_－) zzZ", Name: "dozing"},
	{Glyph: "(ᴗ.ᴗ)", Name: "drowsy"},
	{Glyph: "(-.-)Zzz...", Name: "asleep"},

	// waving / greeting
	{Glyph: "(ovo)/", Name: "wave"},
	{Glyph: "(￣v￣)ノ", Name: "waving hello"},
	{Glyph: "ヾ(＾-＾)ノ", Name: "cheerful wave"},
	{Glyph: "( ' v ` )ﾉ", Name: "friendly wave"},
	{Glyph: "(=^-ω-^=)", Name: "cat wave"},

	// running
	{Glyph: "ᕕ( ᐛ )ᕗ", Name: "running"},
	{Glyph: "εぐ(ノoдo)ノ", Name: "running away"},
	{Glyph: "(ノo口o）ノ", Name: "dashing off"},

	// dance
	{Glyph: "٩(^‿^)۶", Name: "dance joy"},
	{Glyph: "ヽ('v`)/", Name: "dancing"},
	{Glyph: "d/(・o･)\\d", Name: "dance music"},
	{Glyph: "\\(^o^)/", Name: "celebrate"},

	// cat
	{Glyph: "(=^･ω･^=)", Name: "cat face"},
	{Glyph: "(=ↀωↀ=)", Name: "curious cat"},
	{Glyph: "ฅ(^.ω.^)ฅ", Name: "cat paws"},
	{Glyph: "(=„ᆽ„=)", Name: "sleepy cat"},

	// disapproval / disbelief
	{Glyph: "ಠ_ಠ", Name: "disapproval"},
	{Glyph: "(￢_￢)", Name: "side glance"},
	{Glyph: "(-_-)", Name: "unamused"},
	{Glyph: "(ーー;)", Name: "disbelief"},

	// begging / pleading
	{Glyph: "(๑.ㅁ.๑)", Name: "determined plea"},
	{Glyph: "(￣ε￣＠)", Name: "pouty plea"},
	{Glyph: "(⋟﹏⋞)", Name: "pleading"},

	// cute
	{Glyph: "(｡◕‿◕｡)", Name: "cute"},
	{Glyph: "(◍.ᴗ.◍)", Name: "adorable"},
	{Glyph: "(๑>ᴗ<)ﻭ", Name: "cute cheer"},
	{Glyph: "(灬oωo灬)", Name: "cute blush"},

	// sly / smug
	{Glyph: "(￣ω￣)", Name: "sly look"},
	{Glyph: "( ͡o ͜ʖ ͡o)", Name: "lenny face"},
	{Glyph: "(¬‿¬)", Name: "sneaky"},
	{Glyph: "( ¬ω¬)", Name: "smirking"},

	// flirty
	{Glyph: "( ﾟ∀ﾟ)", Name: "cheeky"},
	{Glyph: "(๑'ㅂ`๑)", Name: "flirty"},
	{Glyph: "(⁎>ᴗ<⁎)", Name: "playful"},

	// peace / victory
	{Glyph: "(￣ー￣)ｖ", Name: "peace sign"},
	{Glyph: "(*^-^)v", Name: "victory"},
	{Glyph: "✌(-‿-)✌", Name: "peace"},

	// apology / worry
	{Glyph: "m(_ _)m", Name: "apologize"},
	{Glyph: "(シ. .)シ", Name: "worried"},
	{Glyph: "(￣口￣;)", Name: "oh no"},
	{Glyph: "(๑. - .๑)", Name: "sorry"},

	// eating / drinking
	{Glyph: "(っ-ڡ-ς)", Name: "yummy"},
	{Glyph: "(*￣v￣)b", Name: "delicious"},
	{Glyph: "_(:3」/)_", Name: "give up flop"},

	// coffee / offering
	{Glyph: "( - 3-)", Name: "coffee"},
	{Glyph: "(っ-ω-)っ ☕", Name: "offering coffee"},

	// fighting / determination
	{Glyph: "ᕦ(ò_óv)ᕤ", Name: "flexing"},
	{Glyph: "(๑.ㅂ.)ง✧", Name: "determined"},
	{Glyph: "(ง'-')ง", Name: "fight"},

	// music
	{Glyph: "d('ε｀ )", Name: "humming"},
	{Glyph: "(  ~ω~)ﾉd", Name: "singing"},

	// wizard / magic
	{Glyph: "(ノoⱯo)ノ~*..*.", Name: "wizard"},
	{Glyph: "(∩｀-´)⊃━☆ﾟ.*", Name: "casting spell"},

	// eepy / cold
	{Glyph: "(((・v・)))", Name: "hug"},
	{Glyph: "(っ'ω`c)", Name: "gentle hug"},
	{Glyph: "( -_-)", Name: "tired"},
	{Glyph: "( v_v )", Name: "exhausted"},
}

// kaomojiCommonNames is a curated subset shown as "Common Kaomoji".
var kaomojiCommonNames = map[string]bool{
	"table flip":     true,
	"put table back": true,
	"shrug":          true,
	"meh shrug":      true,
	"happy":          true,
	"laughing":       true,
	"sad":            true,
	"crying":         true,
	"angry":          true,
	"love affection": true,
	"heart eyes":     true,
	"wave":           true,
	"waving hello":   true,
	"dancing":        true,
	"cat face":       true,
	"disapproval":    true,
	"lenny face":     true,
	"peace sign":     true,
	"apologize":      true,
	"confused":       true,
	"surprised":      true,
	"blushing":       true,
	"sleepy":         true,
	"running":        true,
}
