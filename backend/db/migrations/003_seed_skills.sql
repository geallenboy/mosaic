-- +goose Up
-- +goose StatementBegin

-- 内置 Skill 种子数据
INSERT INTO skill_registry (name, type, description, source, output_type, supported_entries, status) VALUES
('Research Skill',  'research',  '市场调研、竞品分析、客群洞察',             'built_in', 'markdown',     ARRAY['documents','sheets'],                            'available'),
('Strategy Skill',  'strategy',  '品牌定位、营销策略、项目主张',             'built_in', 'markdown',     ARRAY['documents','slides'],                            'available'),
('Copy Skill',      'copy',      '宣传文案、社媒内容、活动话术',             'built_in', 'markdown',     ARRAY['documents','slides','videos','podcasts'],        'available'),
('Deck Skill',      'deck',      'PPT 结构、页面文案、提案逻辑',            'built_in', 'markdown',     ARRAY['slides'],                                        'available'),
('Visual Skill',    'visual',    '海报、菜单、社媒图的视觉方向与提示词',       'built_in', 'image_prompt', ARRAY['images','skypage'],                              'available'),
('Web Skill',       'web',       '官网落地页结构与页面内容生成',             'built_in', 'html',         ARRAY['skypage'],                                       'available'),
('Video Skill',     'video',     '短视频脚本、分镜说明、字幕文案',           'built_in', 'markdown',     ARRAY['videos'],                                        'available'),
('Ops Skill',       'ops',       '执行清单、时间节奏、落地动作',             'built_in', 'markdown',     ARRAY['documents','sheets'],                            'available'),
('Dev Skill',       'dev',       '代码生成、API 集成、技术文档',            'built_in', 'code',         ARRAY['ai_developer'],                                  'available');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM skill_registry WHERE source = 'built_in';
-- +goose StatementEnd
