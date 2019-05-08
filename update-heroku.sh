
SERVICENAME="fedor-hasura-test"
git clone http://github.com/hasura/graphql-engine-heroku
cd graphql-engine-heroku
heroku git:remote -a $SERVICENAME
heroku stack:set container -a $SERVICENAME
git push heroku master
cd ..
rm -rf  graphql-engine-heroku

