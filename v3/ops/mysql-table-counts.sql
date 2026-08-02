SET SESSION group_concat_max_len = 16777216;
SELECT CONCAT(
  'SELECT * FROM (',
  GROUP_CONCAT(
    CONCAT('SELECT ', QUOTE(table_name), ' AS table_name, COUNT(*) AS row_count FROM `', REPLACE(table_name, '`', '``'), '`')
    ORDER BY table_name SEPARATOR ' UNION ALL '
  ),
  ') AS sakura_counts ORDER BY table_name'
) INTO @sakura_count_query
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE';
PREPARE sakura_count_statement FROM @sakura_count_query;
EXECUTE sakura_count_statement;
DEALLOCATE PREPARE sakura_count_statement;
