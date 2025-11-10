package end2end

// O版本: 支持精品音色, 联网搜索和RAG, 无法克隆音色 SC版本: 无精品音色, 支持克隆音色
// O版本音色:
//  zh_female_vv_jupiter_bigtts：对应vv音色，活泼灵动的女声，有很强的分享欲
//  zh_female_xiaohe_jupiter_bigtts：对应xiaohe音色，甜美活泼的女声，有明显的台湾口音
//  zh_male_yunzhou_jupiter_bigtts：对应yunzhou音色，清爽沉稳的男声
//  zh_male_xiaotian_jupiter_bigtts：对应xiaotian音色，清爽磁性的男声
// 客户端音频: PCM格式, 单声道, 采样频率16000, 每个采样点用int16表, 小端法
// 服务端音频: OGG封装的的Opus音频(若客户端增加StartSession时间的TTS配置可以返回PCM格式, 单声道, 24000HZ, 32bit位深, 小端法)
// {
//   "tts" : {
//     "audio_config": {
//       "channel": 1,
//       "format": "pcm",
//       "sample_rate": 24000
//     }
//   }
// }
