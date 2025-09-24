-- YYYYmmddHHMM
-- 201804092200 up
-- http://www.sqlite.org/pragma.html#pragma_encoding
PRAGMA encoding = "UTF-8";
PRAGMA temp_store = MEMORY;
-- 'Groups for users in a Слово system.'
CREATE TABLE groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name VARCHAR(100) UNIQUE NOT NULL,
  description VARCHAR(255) NOT NULL,
  disabled INT(1) NOT NULL DEFAULT 1
);

-- 'This table stores the users'
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  login_name varchar(100) UNIQUE,
  -- sha1_sum(encode('UTF-8',$login_name.$login_password))
  login_password varchar(40) NOT NULL,
  first_name varchar(100) NOT NULL DEFAULT '',
  last_name varchar(100) NOT NULL DEFAULT '',
  email varchar(255) NOT NULL UNIQUE,
  description varchar(255) DEFAULT NULL,
  -- 'last modification time'
  -- 'All dates are stored as seconds since the epoch(1970) in GMT.
  -- In Perl we use gmtime as object from Time::Piece'
  tstamp INTEGER DEFAULT 0,
  -- 'registration time',,
  reg_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
  disabled INT(1) DEFAULT 1,
  start_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
  stop_date INTEGER DEFAULT 0
);
CREATE INDEX user_start_date ON users(start_date);
CREATE INDEX user_stop_date ON users(stop_date);


-- 'Which user to which group belongs'
CREATE TABLE user_group (
--  'ID of the user belonging to the group with group_id.'
  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
--  'ID of the group to which the user with user_id belongs.'
  group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  PRIMARY KEY(user_id, group_id)
);

INSERT INTO `groups`(id,name,description) VALUES(0,'null','null group');
INSERT INTO `users` (id,login_name,login_password,first_name,last_name,email,description)
    VALUES(0,'null','9f1bd12057905cf4f61a14e3eeac06bf68a28e64',
    'Null','Null','null@localhost','Disabled system user. Do not use!');
INSERT INTO `user_group` VALUES(0,0);


-- Why we add columns(constraints) later? See https://stackoverflow.com/questions/1884818/#23574053
ALTER TABLE groups ADD COLUMN created_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT;
ALTER TABLE groups ADD COLUMN changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT;

ALTER TABLE users ADD COLUMN group_id INTEGER DEFAULT NULL REFERENCES groups(id) ON DELETE SET DEFAULT;
ALTER TABLE users ADD COLUMN created_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT;
ALTER TABLE users ADD COLUMN changed_by INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE SET DEFAULT;

INSERT INTO groups VALUES(1,'admin','group for administrators',0,0,0);
INSERT INTO users VALUES(1,'foo','9f1bd12057905cf4f61a14e3eeac06bf68a28e64','Foo','Bar','foo@localhost',
  'System user. Do not use!',1,1,0,0,0, 1,0,0);
 INSERT INTO `user_group` VALUES(1,1);

INSERT INTO groups VALUES(2,'guest','guest',0,1,1);
INSERT INTO users
VALUES(2,'guest','8097beb8d5950479e49d803e683932150f469827','гостенин','','guest@localhost',
  'Guest user. Anybody not authenticated is a guest user.',
  1,1,0,0,0, 2,1,1);
INSERT INTO `user_group` VALUES(2,2);

INSERT INTO `groups` VALUES(3,'test1','test1',1,1,1);
INSERT INTO `users` VALUES(3,'test1','b5e9c9ab4f777c191bc847e1aca222d6836714b7','Test','1','test1@localhost',
  'test1 user. Delete. used for tests only.',1,1,1,0,0, 3,1,1);
INSERT INTO `user_group` VALUES(3,3);

INSERT INTO `groups` VALUES(4,'test2','test2',0,1,1);
-- pasword for user test2: sha1_sum(encode("UTF-8","test2test2"))
INSERT INTO `users` VALUES(4,'test2','272a11a0206b949355be4b0bda9a8918609f1ac6','Test','2','test2@localhost',
  'test2 user. Delete. Used for tests only.',
  1,1,0,0,0, 4,1,1);
INSERT INTO `user_group` VALUES(4,4);

INSERT INTO `groups` VALUES(5,'краси','краси',0,4,4);
-- sha1_sum(encode("utf8","красиберов")
INSERT INTO `users` VALUES(5,'краси','f65676423e87854b434b8015da176c01b086ef0b','Краси','Беров','краси@localhost',
    'краси user. Delete this user. ' || 'Used for tests only.',1,1,0,0,0, 5,4,4);
INSERT INTO `user_group` VALUES(5,5);

-- 201804092200 down
DROP TABLE IF EXISTS user_group;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS groups;

-- YYYYmmddHHMM
-- 201804302200 up
-- 'Sites managed by this system'
CREATE TABLE domove (
-- domove is the plural form of 'dom' in Bulgarian, meaning 'home'.
-- The similarity with domains is not a coincidence
--  'Id referenced by stranici that belong to this domain.'
  id INTEGER PRIMARY KEY AUTOINCREMENT,
--  'Domain name as in $ENV{HTTP_HOST}.'
  domain VARCHAR(63) UNIQUE NOT NULL,
--  'The name of this site.'
  site_name VARCHAR(63) NOT NULL,
--  'Site description'
  description VARCHAR(2000) NOT NULL DEFAULT '',
--   'User for which the permissions apply (owner).'
  owner_id INTEGER REFERENCES users(id),
--  'Group for which the permissions apply.'
  group_id INTEGER  REFERENCES groups(id),
--  'Domain permissions',
  permissions VARCHAR(10) DEFAULT '-rwxr-xr-x' ,
--  '0:not published, 1:for review, >=2:published'
  published INT(1) CHECK(published IN(0, 1, 2)) DEFAULT 0
);

CREATE INDEX IF NOT EXISTS domove_published ON domove(published);

INSERT INTO domove (id, domain, description, site_name, owner_id, permissions, published)
    VALUES ( 0, 'localhost', 'default domain', 'Слово', 0, '-rwxr-xr-x', 2);

CREATE TABLE stranici (
  -- 'stranica(страница)' in Bulgarian means 'page'.
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  -- Parent page id
  pid INTEGER DEFAULT 0,
  -- Refrerence to domove.id to which this page belongs.
  dom_id INTEGER DEFAULT 0,
  -- Alias for the page which may be used instead of the id.
  alias VARCHAR(32) NOT NULL,
  -- 'обичайна','коренъ' etc.
  page_type VARCHAR(32) NOT NULL,
  -- Page editing permissions. Prefixes: d - folder, l - link
  permissions varchar(10) DEFAULT '-rwxr-xr-x',
  sorting INTEGER DEFAULT 1,
  -- MT code to display this page. Default template is used if not specified.
  template VARCHAR(255),
  -- User for which the permissions apply (owner/creator).
  user_id INTEGER,
  -- Group for which the permissions apply (usually primary group of the owner).
  group_id INTEGER,
  tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
  start TIMESTAMP DEFAULT CURRENT_TIMESTAMP, 
  stop TIMESTAMP DEFAULT 0,
  -- 0: not published, 1: for review/preview, >=2: published
  published INT(1) DEFAULT 1,
  -- Is this page hidden? 0=No, 1=Yes
  hidden INT(1) DEFAULT 0,
  -- Is this page deleted? 0=No, 1=Yes
  deleted INT(1) DEFAULT 0,
  -- Who modified this page the last time?
  changed_by INTEGER REFERENCES users(id),
  FOREIGN KEY (pid)       REFERENCES stranici(id) ON UPDATE CASCADE ON DELETE CASCADE,
  FOREIGN KEY (dom_id)    REFERENCES domove(id)   ON UPDATE CASCADE ON DELETE CASCADE,
  FOREIGN KEY (user_id)   REFERENCES users(id)    ON UPDATE CASCADE,
  FOREIGN KEY (group_id)  REFERENCES groups(id)   ON UPDATE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS stranici_alias_in_domove ON stranici(alias, dom_id);
CREATE INDEX IF NOT EXISTS stranici_user_id_group_id ON stranici(user_id, group_id);
CREATE INDEX IF NOT EXISTS stranici_hidden ON stranici(hidden);

-- only root pages have pid same as id!
INSERT INTO stranici (
    id, alias, changed_by, deleted, dom_id, group_id, hidden, page_type,
    permissions, pid, published, sorting, start, stop, tstamp, user_id)
VALUES (
    0, 'коренъ', 0, 0, 0, 0, 0, 'root',
    '-rwxr-xr-x', 0, 1, 0, 0, 0, 1523795424, 0);


 -- Initially created by SQL::Translator::Producer::SQLite
 -- Created on Sat Apr 14 13:32:46 2018
 --

 CREATE TABLE celini (
 -- content elements are one or more paragraphs, or whole article. Different
 -- data_types denote the semantic of a content element.
 -- This table is a modified version of MYDLjE table "content".
 -- 'celina(цѣлина)' is the original Bulgarian word for 'paragraph' – едно цѣло ѿ слова.

  -- Primary unique identifier
  id INTEGER PRIMARY KEY,
  -- Lowercased and trimmed of \W characters unique identifier for the row data_type
  alias VARCHAR(255) DEFAULT 'seo-friendly-id',
  -- Parent content: Question, Article, Note, Book ID etc.
  pid INTEGER DEFAULT 0,
  -- Id from which this content is copied (translated), if not original content.
  from_id INTEGER DEFAULT 0,
  -- page.id to which this content belongs. Default: 0
  page_id INTEGER DEFAULT 0,
  -- User for which the permissions apply (owner).
  user_id INTEGER NOT NULL,
  -- Group for which the permissions apply.(primary group of the user by default)
  group_id INTEGER NOT NULL,
  -- For sorting chapters in a book, stranici in a menu etc.
  sorting int(10) DEFAULT 0,
  -- Semantic content types: 'question', 'answer', 'writing', 'note',
  -- 'book', 'глава', 'title', 'paragraph'…
  data_type VARCHAR(32) CHECK(data_type IN(
        'question', 'answer', 'writing', 'note', 'book', 'глава', 'title',
        'paragraph')) DEFAULT 'note',
  -- text, html, markdown, asc…
  data_format VARCHAR(32) CHECK(data_format IN('text', 'html', 'markdown')) DEFAULT 'text',
  -- When this content was inserted
  created_at INTEGER NOT NULL DEFAULT 0,
  -- Last time the record was touched
  tstamp INTEGER DEFAULT 0,
  -- Used in title html tag for stranici or or as h1 for other data types.
  title VARCHAR(255) DEFAULT '',
  -- Used in description meta tag when appropriate.
  description VARCHAR(255) DEFAULT '',
  -- Used in keywords meta tag.
  keywords VARCHAR(255) DEFAULT '',
  -- Used in tag cloud boxes. merged with keywords and added to keywords meta tag.
  tags VARCHAR(100) DEFAULT '',
  -- Main celini when applicable.
  body TEXT DEFAULT '',
  -- celini box in which this element should be displayed (e.g. main|главна, left|лѣва, right|дѣсна, header|глава, footer|дъно, foo, bar).
  box VARCHAR(35) CHECK(box IN('main', 'главна', 'left', 'лѣва', 'right', 'дѣсна',
        ' header', 'глава', 'footer', 'дъно')) DEFAULT 'main',
  -- Language of this content. Means any language, when empty string
  language VARCHAR(5) DEFAULT '',
  -- tuuugggooo - Experimental permissions for the content. Who can see/edit/delete it.
  -- TODO: document and design the behavior for pages which are "d" (directories) and "l" (links)
  permissions char(10) DEFAULT '-rwxr-xr-x',
  -- Show on top independently of other sorting.
  featured int(1) CHECK(featured IN(0, 1)) DEFAULT 0,
  -- Answer accepted?
  accepted int(1) CHECK(accepted IN(0, 1)) DEFAULT 0,
  -- Reported as inapropriate offensive etc. Higher values mean "very bad".
  bad int(2) DEFAULT 0,
  -- When set to 1 the record is not visible anywhere.
  deleted int(1) CHECK(deleted IN(0, 1)) DEFAULT 0,
  -- Date/Time from which the record will be accessible in the site.
  start TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  -- Date/Time till which the record will be accessible in the site.
  stop TIMESTAMP DEFAULT 0,
  -- Who modified this record the last time?
  changed_by INTEGER REFERENCES users(id),
  FOREIGN KEY (pid)      REFERENCES celini(id)   ON UPDATE CASCADE ON DELETE CASCADE,
  FOREIGN KEY (page_id)  REFERENCES stranici(id) ON UPDATE CASCADE ON DELETE CASCADE,
  FOREIGN KEY (user_id)  REFERENCES users(id)    ON UPDATE CASCADE,
  FOREIGN KEY (group_id) REFERENCES groups(id)   ON UPDATE CASCADE
 );

 CREATE INDEX celini_pid ON celini (pid);
 CREATE INDEX celini_tags ON celini (tags);
 CREATE INDEX celini_user_id_group_id ON celini (user_id, group_id);
 CREATE INDEX celini_data_type ON celini (data_type);
 CREATE INDEX celini_language ON celini (language);
 CREATE INDEX celini_page_id ON celini (page_id);
 CREATE INDEX celini_deleted ON celini (deleted);
 CREATE UNIQUE INDEX celini_alias_with_data_type_in_page_id ON celini (alias, data_type, page_id);

 CREATE INDEX user_group_id ON users(group_id);

  INSERT INTO celini (
      id, alias, body, created_at, data_format, data_type, group_id,
      keywords, language, page_id, pid, tags, title, user_id)
  VALUES (
      0, 'начало', '', 1523807733, 'text', 'title', 0,
      'Slovo, Слово', 'bg', 0, 0, 'начало, home', 'Слово', 0);

               

-- 201804302200 down
DROP TABLE IF EXISTS domove;
DROP TABLE IF EXISTS stranici;
DROP TABLE IF EXISTS celini;
-- time passes....

-- 202102010000 up
-- Make stricter constrains for stranici and celini.
-- Disalow accidential equallity between .pid and .id and thus deadly
-- recursion.
-- All celini which data_type=title  (are used as title of a page) must be
-- containers `permissions LIKE 'd%'`!
-- All celini which contain other celini must be containers`permissions LIKE 'd%'`!
-- All pages which will contain other pages must have `permissions LIKE 'd%'`.
CREATE TRIGGER stranici_bu BEFORE UPDATE ON stranici
BEGIN
    SELECT
        CASE
            -- a page cannot be parent to itself
            -- except the root of all roots
            WHEN NEW.pid = OLD.id AND NEW.pid !=0
            THEN RAISE(ABORT, 's.pid cannot be equal to s.id')
            -- The parent page must be a container.
            WHEN EXISTS (SELECT permissions FROM stranici WHERE id=NEW.pid AND permissions NOT LIKE 'd%')
            THEN RAISE(ABORT, 'The parent page must be a directory(container). Change its permisssions to start with "d"!')
        END;
END;
UPDATE celini SET permissions = 'drwxrwxr-x' where id=0;
UPDATE celini SET permissions = 'drwxrwxr-x' where data_type='title';

CREATE TRIGGER celini_bu BEFORE UPDATE ON celini
BEGIN
    SELECT
        CASE
            -- a celina cannot be parent to itself
            -- except the one with page_id=0
            WHEN NEW.pid = OLD.id AND NEW.pid !=0
            THEN RAISE(ABORT, 'c.pid cannot be equal to c.id' )
            -- The parent celina must be a container.
            WHEN EXISTS (SELECT permissions FROM celini WHERE id=NEW.pid AND permissions NOT LIKE 'd%')
            THEN RAISE(ABORT, 'The parent celina must be a directory(container). Change its permisssions to start with "d"!')
        END;
END;

-- 202102010000 down
DROP TRIGGER IF EXISTS stranici_bu;
DROP TRIGGER IF EXISTS celini_bu;


-- 202509172303 up
CREATE TABLE other_types (
-- This is a dummy table to just cover the cases for type generation for Go,
-- found in rx/utils.go:sql2GoTypeAndTag

id ENUM PRIMARY KEY,
yes_or_no BOOL NOT NULL DEFAULT 'false',
published TINYINT NOT NULL DEFAULT 0,
states SMALLINT NOT NULL DEFAULT 0,
big BIGINT NOT NULL DEFAULT 0,
byte BYTEA NOT NULL DEFAULT 0x00,
float4 FLOAT4 NOT NULL DEFAULT 0.0,
deci DECIMAL NOT NULL DEFAULT 0.000,
unkn UNKNOWN NOT NULL DEFAULT 0
);

-- 202509172303 down
DROP TABLE IF EXISTS other_types;
