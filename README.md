# linkit

a simple web service to share personal links


### database setup

    sqlite3 linkit.db
    .headers on
    .mode columns

    create table category (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, url_stub TEXT, passcode TEXT, page_top_blurb TEXT);
    insert into category (name, url_stub, passcode, page_top_blurb) values ('Foto', 'foto', 'secret', 'This is a list of links for <a href="https://hawaii.slack.com/archives/CJDLL15QB" target="_blank">the Hawaii #photography slack channel</a>.');
    insert into category (name, url_stub, passcode, page_top_blurb) values ('Socia', 'socia', 'secret', 'Shared links for the Hawaii slack #socialmedia channel.');

    create table link (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, name TEXT, url TEXT, notes TEXT, added TEXT, safe INTEGER DEFAULT 0);
    insert into link (category,name,url,notes) values ('Hawaii', 'Patrick D Kelly', 'https://instagram.com/phlatphrog', 'taking the scenic route');
    insert into link (category,name,url,notes) values ('Hawaii', 'Ryan Ozawa', 'https://www.instagram.com/hawaii/', 'online social media mastermind');

    create table google (api_key);
    insert into google values ('blahblahfoofoo');