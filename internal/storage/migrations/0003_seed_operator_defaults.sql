-- Реквизиты оператора по умолчанию и общий счётчик номеров.
-- INSERT OR IGNORE: уже заданные через бота значения не перезаписываются.
INSERT OR IGNORE INTO operator_settings (key, value) VALUES
    ('full_name',      'Общество с ограниченной ответственностью «СТЕЙБЛЕКС»'),
    ('short_name',     'ОсОО «СТЕЙБЛЕКС»'),
    ('inn',            '9909730748'),
    ('kpp',            '770387001'),
    ('address',        'Кыргызская Республика, г. Бишкек, ул. Московская, д. 197'),
    ('legal_address',  '720009, г. Бишкек, ул. Московская, д. 197'),
    ('license',        'VA №0171'),
    ('kio',            '73074'),
    ('inn_rf',         '9909730748'),
    ('kpp_rf',         '770387001'),
    ('bank_name',      'КБ "Долинск" (АО)'),
    ('bank_account',   '40807810500014264602'),
    ('bank_bik',       '046401727'),
    ('wallet',         'TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r'),
    ('director_name',  'Зенков И.В.'),
    ('director_title', 'Директор');

INSERT OR IGNORE INTO act_counter (id, last_number) VALUES (1, 499);
