# The stages of CTP's sql/bin/run.sh, as functions the runner calls one at a
# time. The bodies are run.sh's (develop a1bec87), kept close enough to diff:
# what the engine's files end up holding, what the database is made of, and
# what a stage prints are CTP's, because a verdict can depend on any of them.
#
# Four things differ, and each is said where it happens. run.sh's globals come
# from the runner, which has already read the configuration and cubrid_rel, in
# front of this file. `ini` is a function rather than run.sh's alias, which
# bash does not expand in a script. HA mode is decided by ha_mode_of rather
# than a global that config_cubrid_ha sets, because a slot starts its server in
# a later invocation than the one that configured the engine. And do_clean's
# two account-wide steps run only inside containment -- see sweep_this_account.
#
# Variables the runner sets: testkit_contained CTP_HOME config_file_main scenario_category
# scenario_full_name db_name cubrid_bits cubrid_ver cubrid_ver_p1 cubrid_ver_p2
# cubrid_ver_p3 cubrid_ver_p4 cubrid_ver_prefix os_type db_charset
# cubrid_createdb_opts need_make_locale test_data_file log_filename
# cubrid_root_dir conf_ha_mode jdbc_config_file_ext

ini() { sh "${CTP_HOME}/bin/ini.sh" "$@"; }

is_support_ha="no"
if [ `cubrid|grep heartbeat|grep -v grep|wc -l` -ne 0 ]; then
     is_support_ha="yes"
fi
support_javasp=`echo $(cubrid | grep -w javasp) | awk '{if($1=="javasp") {print "yes"} else {print "no"}}'`

# do_configure's environment. run.sh exports it partway through and every later
# command inherits it; here every stage from configure on starts with it.
sql_env()
{
     java_version=`file $JAVA_HOME/bin/java|grep 64-bit|wc -l`
     if [ $java_version -ne 0 ];then
          LD_LIBRARY_PATH=$JAVA_HOME/jre/lib/amd64:$JAVA_HOME/jre/lib/amd64/server:$LD_LIBRARY_PATH
     else
          LD_LIBRARY_PATH=$JAVA_HOME/jre/lib/i386/:$JAVA_HOME/jre/lib/i386/client:$LD_LIBRARY_PATH
     fi
     export LD_LIBRARY_PATH
     export LC_ALL=en_US.UTF-8
     export CUBRID_CHARSET=$db_charset
     export CUBRID_LANG=en_US
}

# Whether the database runs under heartbeat: config_cubrid_ha's condition.
ha_mode_of()
{
     cnt=`cat $CUBRID/conf/cubrid.conf | grep -v "#" | grep ha_mode | grep -E 'on|yes' | wc -l `
     if [ "$is_support_ha" == "yes" ] && { [ "$conf_ha_mode" == "yes" ] || [ "$conf_ha_mode" == "on" ]; } && [ "$cnt" -gt 0 ]
     then
          echo yes
     else
          echo no
     fi
}

clean_log_cores()
{
     rm -rf "$CUBRID/logs/*" 2>&1 > /dev/null
     find "$CUBRID" "${CTP_HOME}" -type f -name "core*"|xargs -i rm {}
}

do_clean()
{
     #stop process
     stop_db $db_name

     #delete database
     delete_db $db_name

     #clean log files and core
     clean_log_cores

     #kill cub for the current user, and this user's shared memory with it
     sweep_this_account

     #reset cubrid conf files
     reset_cubrid_files
}

# sweep_this_account is run.sh's `pkill cub` and its ipcrm loop, behind the one
# condition that makes them true.
#
# Both select by account and not by run: `pkill cub` matches every CUBRID
# process the user owns, and remove_shared_memory hands `ipcrm` every segment
# `ipcs` attributes to $USER. On a machine given over to one run that is what
# cleaning up means, and it is why run.sh does it. On a machine that is not,
# it reaches whatever else the account is running -- another suite's databases,
# a broker somebody is using, and this project's own sandbox nodes, whose
# CUBRID processes are ordinary processes of the same user.
#
# Measured here on 2026-09-22: every CUBRID process on this host stopped twice
# in one evening, both sandbox clusters and the host's own install, in the same
# millisecond each time. A peer session ran this suite about ten times in that
# window **with** TESTKIT_CONTAIN=1 -- each run reaching this function -- and
# nothing of theirs touched anything of ours. Containment makes the account and
# the run the same thing: its own PID and IPC namespaces are exactly the scope
# these two commands assume they have.
#
# So they run when that is true and say why when it is not. The alternative
# considered and rejected was narrowing the selection to the run's own
# processes, which is a second model of what belongs to a run and would drift
# from run.sh silently; refusing is visible and keeps the body diffable.
sweep_this_account()
{
     if [ "$testkit_contained" != "1" ]; then
          echo "do_clean: leaving this account's cub processes and shared memory alone," \
               "because the run is not contained -- set TESTKIT_CONTAIN=1 for run.sh's behaviour"
          return 0
     fi
     pkill cub
     remove_shared_memory
}

remove_shared_memory()
{
     for x in `ipcs -a|grep $USER|awk '{print $2}'`
     do
         ipcrm -m $x
     done
}

reset_cubrid_files()
{
     curDir=`pwd`
     name="forFun"
     cd $CUBRID/conf
     if [ -f "cubrid_broker.conf.$name" ];then
     	cp cubrid.conf.$name cubrid.conf
     else
   	cp cubrid.conf cubrid.conf.$name
     fi

     if [ -f "cubrid_broker.conf.$name" ];then
     	cp cubrid_broker.conf.$name cubrid_broker.conf
     else
   	cp cubrid_broker.conf cubrid_broker.conf.$name
     fi

     if [ -f "cubrid_ha.conf.$name" ]; then
           cp cubrid_ha.conf.$name cubrid_ha.conf
     else
   	cp cubrid_ha.conf cubrid_ha.conf.$name
     fi

     cd $curDir
}

config_cubrid_without_ha()
{
     cubrid_conf_para=`ini -s "sql/cubrid.conf" --separator="||" ${config_file_main}`

     # [CBRD-26220] In CUBRID 11.5+, the java_stored_procedure parameters are identical to PL,
     # no longer documented, and marked as DEPRECATED.
     # Therefore, they are filtered out in CTP for 11.5+.
     if [ "${cubrid_ver_p1}" -gt 11 ] || { [ "${cubrid_ver_p1}" -eq 11 ] && [ "${cubrid_ver_p2}" -ge 5 ]; }; then
        echo "Filtering out deprecated parameters for CUBRID 11.5+"
        cubrid_conf_para=`echo "$cubrid_conf_para" | sed 's/java_stored_procedure=[^|]*||*//g' | sed 's/||*$//'`
     fi

     if [ "$cubrid_conf_para" ];then
     	ini -s common -u "${cubrid_conf_para}" $CUBRID/conf/cubrid.conf
     fi
     ini -s service -u "service=server,broker" $CUBRID/conf/cubrid.conf
     cubrid_broker_shm=`ini -s "sql/cubrid_broker.conf/broker" --separator="||" ${config_file_main}`
     if [ "$cubrid_broker_shm" ];then
     	ini -s "broker" -u $cubrid_broker_shm $CUBRID/conf/cubrid_broker.conf
     fi

     cubrid_broker_conf_para=`ini -s "sql/cubrid_broker.conf/%BROKER1" --separator="||" ${config_file_main}`
     cubrid_broker_conf_queryeditor_para=`ini -s "sql/cubrid_broker.conf/%query_editor" --separator="||" ${config_file_main}`
     is_valid_section_broker1=`cat $CUBRID/conf/cubrid_broker.conf|grep '\[\%BROKER1\]'|grep -v '#'|wc -l`
     is_valid_section_queryeditor=`cat $CUBRID/conf/cubrid_broker.conf|grep '\[\%query_editor\]'|grep -v '#'|wc -l`

     if [ "$cubrid_broker_conf_queryeditor_para" ] && [ $is_valid_section_queryeditor -ne 0 ];then
     	ini -s "%query_editor" -u "${cubrid_broker_conf_queryeditor_para}" $CUBRID/conf/cubrid_broker.conf
     fi

     if [ "$cubrid_broker_conf_para" ] && [ $is_valid_section_broker1 -ne 0 ];then
     	ini -s "%BROKER1" -u "${cubrid_broker_conf_para}" $CUBRID/conf/cubrid_broker.conf
     fi
}

# run.sh's jdbc branch. The CCI branch is sql_by_cci's, which stays with CTP.
config_qa_tool()
{
     curDir=`pwd`
     build_ver_type=""
     qa_db_xml_path=${CTP_HOME}/sql/configuration/Function_Db/${db_name}_qa.xml
     avaliable_broker_port=`awk '/SERVICE[[:space:]]*=[[:space:]]*ON/, /BROKER_PORT/' $CUBRID/conf/cubrid_broker.conf|grep BROKER_PORT|grep -v '#'|awk -F '=' '{print $2}'|head -1| tr -d ' ' | tr -d '\r'`
     db_url="<dburl>jdbc:cubrid:localhost:${avaliable_broker_port}:${db_name}:::</dburl>"
     sed -i "s#<dburl>.*</dburl>#$db_url#g" $qa_db_xml_path
     if [ "$cubrid_bits" == "32" ];then
          build_ver_type="32bits"
     else
          build_ver_type="Main"
     fi
     sed -i "s#<version>.*</version>#<version>$build_ver_type</version>#g" $qa_db_xml_path
     cd $curDir
}

config_cubrid_ha()
{
     echo "start config ha"
     if [ "$is_support_ha" != "yes" ];then
        return
     fi

     if [ `ha_mode_of` == "yes" ]
     then
          cubrid_ha_para=`ini -s "sql/cubrid_ha.conf" ${config_file_main} --separator="||"`
          ini -s common -u "ha_db_list=$db_name||$cubrid_ha_para" $CUBRID/conf/cubrid_ha.conf
     fi
}

make_locale()
{
     curDir=`pwd`
     if [ "$need_make_locale" == "no" ];then
        echo "Don't need  make locale since you configure need_make_locale=no !"
        return
     fi

     version_type=`cubrid_rel | grep debug | wc -l`

     if [ $cubrid_ver_p1 -eq 8 ]; then
             if [ $cubrid_ver_p2 -lt 4 ]; then
                     echo 1
                     return
             else
                     if [ $cubrid_ver_p3 -lt 9 ]; then
                             echo 1
                             return
                     fi
             fi
     fi

     echo "make locale now"
     mv $CUBRID/conf/cubrid_locales.txt $CUBRID/conf/cubrid_locales_bak.txt
     cp $CUBRID/conf/cubrid_locales.all.txt $CUBRID/conf/cubrid_locales.txt
     cd $CUBRID/bin

     if [ $version_type -eq 0 ]
     then
          sh make_locale.sh -t $cubrid_bits
     else
          sh make_locale.sh -t $cubrid_bits -m debug
     fi

     cd $curDir
}

do_configure()
{
     curDir=`pwd`
     sql_env

     #set cubrid conf
     config_cubrid_without_ha

     #config HA env, to check if need ha mode, if need it, config it
     config_cubrid_ha

     #config qa tool
     config_qa_tool

     #make locale
     make_locale

     #config QA tool
     if [ "$cubrid_ver_p4" -a "$cubrid_ver_prefix" ]
     then
          ini -u "dbbuildnumber=$cubrid_ver_p4||dbversion=$cubrid_ver_prefix" ${CTP_HOME}/sql/configuration/local.properties
     fi

     cd $curDir
}

stop_db()
{
     echo "stop database $1"
     cnt=`cat $CUBRID/conf/cubrid.conf | grep -v "#" | grep ha_mode | grep -E 'on|yes' | wc -l `
     if [ "$is_support_ha" == "yes" -a $cnt -gt 0 ]
     then
         cubrid hb stop $1 2>&1 > /dev/null
     else
         cubrid server stop $1 2>&1 > /dev/null
     fi

     jcnt=`cat $CUBRID/conf/cubrid.conf | grep -v "#" | grep java_stored_procedure | grep -E 'on|yes' | wc -l`
     if [ $support_javasp == "yes" -a $jcnt -gt 0 ]
     then
          cubrid javasp stop $1 2>&1 >> $log_filename
          echo "stop javasp database $1"
     fi

     sleep 2
     cubrid service stop 2>&1 > /dev/null
}

do_create_db()
{
     sql_env
     echo "MAKE $db_name DATABASE (default size)..."
     curDir=`pwd`
     mkdir -p ${cubrid_root_dir}/databases
     cd $cubrid_root_dir/databases
     if [ ! -d $db_name ]
     then
  	mkdir -p $db_name
  	cd $db_name
     else
  	rm -rf $db_name/* 2>&1 > /dev/null
          cd $db_name
     fi

     if [ $cubrid_ver_p1 -ge 9 -a $cubrid_ver_p2 -gt 1 ] || [ $cubrid_ver_p1 -ge 10 ]
     then
          echo "cubrid createdb $cubrid_createdb_opts $db_name $db_charset"
          cubrid createdb $cubrid_createdb_opts $db_name $db_charset 2>&1 >> $log_filename
     else
          echo "cubrid createdb $cubrid_createdb_opts $db_name"
          cubrid createdb $cubrid_createdb_opts $db_name 2>&1 >> $log_filename
     fi

     if [ "${scenario_full_name}" == "medium" ];then
          make_db_data mdb
     else
	  make_sql_db_data
     fi

     cd $curDir
}

optimize_db()
{
     echo "optimizedb $1..."
     cubrid optimizedb $1 2>&1 >> $log_filename
     sleep 1
}

start_db()
{
     echo "start database $1"
     cubrid service stop
     sleep 1
     cubrid service start
     sleep 1
     echo "start database $1"
     cnt=`cat $CUBRID/conf/cubrid.conf | grep -v "#" | grep ha_mode | grep -E 'on|yes' | wc -l `
     if [ "$cnt" -gt 0 ]
     then
         cubrid hb start $1 2>&1 >> $log_filename
     else
         cubrid server start $1 2>&1 >> $log_filename
     fi

     jcnt=`cat $CUBRID/conf/cubrid.conf | grep -v "#" | grep java_stored_procedure | grep -E 'on|yes' | wc -l`
     if [ $support_javasp == "yes" -a $jcnt -gt 0 ]
     then
          cubrid javasp start $1 2>&1 >> $log_filename
          echo "start javasp database $1"
     fi

     sleep 2
}

delete_db()
{
     echo "delete database $1"
     cubrid deletedb $1 2>&1 >> $log_filename
     sleep 2

     #delete db folder
     cd $cubrid_root_dir/databases
     if [ -d "$db_name" ];then
		rm -rf $db_name
     fi
}

restart_broker()
{
     echo "restart broker..."
     cubrid broker restart 2>&1 >> $log_filename
     sleep 2
}

make_sql_db_data()
{
     curDir=`pwd`
     echo "Load Java Stored Procedure Classes"
     cd ${CTP_HOME}/sql/function/stored_procedure/src
     rm *.class 2>&1 > /dev/null
     "$JAVA_HOME/bin/javac" -cp $CUBRID/jdbc/cubrid_jdbc.jar *.java

     for clz in $(ls *.class);do
 	echo "Load ${clz}..."
 	loadjava $db_name $clz 2>&1 >> $log_filename
     done

     rm *.class 2>&1 >/dev/null
     cd $curDir
}

check_status()
{
    db_name=$1
    if [ `ha_mode_of` == "yes" ];then
        res=`cubrid changemode $db_name 2>&1`
        res=`echo $res | grep -e "active" -e "not configured for HA"`
        m=0
        while [ -z "$res" ]; do
             if [ $m -gt 50 ]; then
                     break
             else
                     sleep 2
                     echo "waiting for $db_name become active..."
                     res=`cubrid changemode $db_name`
                     res=`echo $res | grep "active"`
                     m=`expr $m + 1`
             fi
        done
    else
   	   server_status=`cubrid server status|grep $db_name|grep -v grep|wc -l`
   	   broker_status=`cubrid broker status|grep PID|wc -l`
   	   m=0
   	   while [ $server_status -ne 1 -o $broker_status -eq 0 ]; do
   	          if [ $m -gt 50 ]; then
   	                  break
   	          else
   	                  sleep 2
   	                  echo "waiting for server and broker become active..."
   	  		server_status=`cubrid server status|grep $db_name|grep -v grep|wc -l`
   	  		broker_status=`cubrid broker status|grep PID|wc -l`
   	                  m=`expr $m + 1`
   	          fi
   	   done
     fi
}

# The server, the broker, and waiting for both: do_test's first three lines.
do_serve()
{
     sql_env
     start_db $db_name
     restart_broker
     check_status $db_name
}

make_db_data()
{
     curDir=`pwd`
     dataFileName=$1
     echo "Load Initial Data..."
     if [ -z "$test_data_file" ];then
        echo "Not found data file(the current is $test_data_file) to load,
              please set the correct value for data_file in $config_file_main"
        exit 1
     fi
     data_file=""
     if [ -d $test_data_file ]
     then
         data_file=${test_data_file}/${dataFileName}.tar.gz
         if [ ! -f $data_file ];then
             echo "please confirm your data file exist(the current directory $test_data_file) does not include tar! "
             exit 1
         fi

     elif [ -f $test_data_file ]
     then
         data_file=$test_data_file
     else
         echo "Not found data file(the current is $test_data_file) to load,
               please set the correct value for data_file in $config_file_main"
         exit 1
     fi

     cd $cubrid_root_dir/databases/$db_name
     cp $data_file .

     tar -zxvf mdb.tar.gz
     loaddb=`cubrid loaddb 2>&1`
     if [[ $loaddb =~ "--no-user-specified-name" ]];then
        cubrid loaddb -s ${db_name}_schema -i ${db_name}_indexes -d ${db_name}_objects -u dba ${db_name} --no-user-specified-name >> $log_filename
     else
        cubrid loaddb -s ${db_name}_schema -i ${db_name}_indexes -d ${db_name}_objects -u dba ${db_name} >> $log_filename
     fi
     # Not run.sh's: it goes on, and every case then fails against a database
     # that was never loaded -- 579 of 975 with medium.conf on an 11.x engine,
     # reported nowhere as a load failure (evidence/sql-baseline.md §3).
     loaddb_status=$?
     if [ $loaddb_status -ne 0 ]; then
        echo "cubrid loaddb ${db_name} failed (exit $loaddb_status); its messages are in $log_filename"
        exit 1
     fi
     optimize_db $db_name

     rm *.gz 2>&1 >/dev/null
     cd $curDir
}

# do_summary_and_clean's core search: every core under the directories named --
# run.sh's are "$CUBRID" "${CTP_HOME}" -- printed as CORE_FILE:<path>. Prints
# nothing when there are none.
find_cores()
{
     coreFiles=$(find "$@" -type f -name "core*")
     while read -r file;
     do
         [ -z "$file" ] && continue
	 isCore=`file "$file"|grep 'core file'|grep -v grep|wc -l`
         if [ $isCore -ne 0 ];then
             echo "CORE_FILE:$file"
         fi
     done <<EOF
$coreFiles
EOF
}
