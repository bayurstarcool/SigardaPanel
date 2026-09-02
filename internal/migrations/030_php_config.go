package migrations

// PHPConfigSchema adds per-site PHP version and custom php.ini settings.
// - php_version: e.g. "8.1", "8.4" — determines which PHP-FPM pool to proxy to (127.0.0.1:{port})
// - php_ini: custom PHP ini directives sent via fastcgi_param PHP_VALUE
// Default empty = use CloudPanel default socket (clp-php-fpm.sock).
var PHPConfigSchema = Migration{
	Version: 30,
	SQL: `ALTER TABLE sites ADD COLUMN php_version TEXT DEFAULT '';
ALTER TABLE sites ADD COLUMN php_ini TEXT DEFAULT '';`,
}
