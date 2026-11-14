-- Catálogo de licenciamento por pontos exportado do portal anterior em
-- 03/11/2026. Modelos sem consumo cadastrado ficam pendentes.

INSERT INTO licensing_programs (key, manufacturer, name, price_usd) VALUES
    ('fortinet', 'Fortinet', 'FortiFlex', 0.96),
    ('sonicwall', 'SonicWall', 'FlexSpend Credits', 0.68);

INSERT INTO licensing_models (program_key, model, segment, sort_order, daily_points, service_description) VALUES
    ('fortinet', 'FortiGate 30G', 'Entrada', 1, NULL, ''),
    ('fortinet', 'FortiGate 40F', 'Entrada', 2, 0.92, 'Licenciamento pleno'),
    ('fortinet', 'FortiGate 50G', 'Pequenas empresas', 3, NULL, ''),
    ('fortinet', 'FortiGate 60F', 'Pequenas empresas', 4, NULL, ''),
    ('fortinet', 'FortiGate 70G', 'Pequenas empresas', 5, NULL, ''),
    ('fortinet', 'FortiGate 90G', 'Pequenas/médias empresas', 6, NULL, ''),
    ('fortinet', 'FortiGate 120G', 'Médias empresas', 7, NULL, ''),
    ('fortinet', 'FortiGate 200G', 'Corporativo', 8, NULL, ''),
    ('fortinet', 'FortiGate 400G', 'Corporativo', 9, NULL, ''),
    ('fortinet', 'FortiGate 700G', 'Corporativo / Datacenter', 10, NULL, ''),
    ('sonicwall', 'TZ80', 'Entrada', 1, NULL, ''),
    ('sonicwall', 'TZ280', 'Entrada', 2, 0.46, 'Pacote completo'),
    ('sonicwall', 'TZ380', 'Pequenas empresas', 3, NULL, ''),
    ('sonicwall', 'TZ480', 'Pequenas empresas', 4, NULL, ''),
    ('sonicwall', 'TZ580', 'Pequenas/médias empresas', 5, NULL, ''),
    ('sonicwall', 'TZ680', 'Médias empresas', 6, NULL, ''),
    ('sonicwall', 'NSa 2800', 'Corporativo', 7, NULL, ''),
    ('sonicwall', 'NSa 3800', 'Corporativo', 8, NULL, ''),
    ('sonicwall', 'NSa 4800', 'Corporativo', 9, NULL, ''),
    ('sonicwall', 'NSa 5800', 'Corporativo / Datacenter', 10, NULL, '');
