import com.navercorp.cubridqa.cqt.common.SQLParser;
import com.navercorp.cubridqa.cqt.console.bean.CaseResult;
import com.navercorp.cubridqa.cqt.console.bean.Sql;
import com.navercorp.cubridqa.cqt.console.bean.Test;
import com.navercorp.cubridqa.cqt.console.bo.ConsoleBO;
import com.navercorp.cubridqa.cqt.console.dao.ConsoleDAO;
import com.navercorp.cubridqa.cqt.console.util.ConfigureUtil;
import com.navercorp.cubridqa.cqt.console.util.JunitXmlWriter;
import com.navercorp.cubridqa.cqt.console.util.PropertiesUtil;
import com.navercorp.cubridqa.cqt.console.util.SystemUtil;
import com.navercorp.cubridqa.cqt.console.util.TestUtil;
import java.io.BufferedReader;
import java.io.File;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.io.PrintWriter;
import java.io.StringWriter;
import java.lang.reflect.Field;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * CQT without its loop. testkit's sql runner starts one of these per slot, sets
 * a run up exactly as ConsoleAgent.runTest does, and then hands it one case at
 * a time. What comes back is the text CQT would have written to that case's
 * .result; judging it, and everything written about it, is the runner's.
 *
 *   java TestkitExecutor <type> <typeAlias> <version> <charset_xml> <files...>
 *
 * The arguments are runCQT's, after the word runCQT, so the command line reads
 * like the one run.sh builds.
 *
 * The run's result directory is the system property testkit.result_dir, so
 * that every CaseResult points where the runner writes -- a failure's text
 * reads the .result the runner copied there.
 *
 * The protocol is on file descriptor 3, which the runner points at its pipe;
 * standard output is CQT's and the JVM's (a thread dump, a -Xlog line), and a
 * stray line there must not be read as a reply to the next case. First
 * "READY <n> <bytes>", n being the cases
 * CQT's own discovery found, and then that many bytes: what CQT printed while
 * it started, which a run prints after "Result Root Dir:" as CQT does; or
 * "E <message>" and exit 2. Then one request per line on standard input:
 *
 *   X <case>   run it: "R <bytes> <ms>" and the rendering, UTF-8, with the
 *              milliseconds CQT measured for it
 *   F <case>   a failed case's text for the JUnit report: "R <bytes> 0" and
 *              the text, from CQT's own JunitXmlWriter
 *
 * or "E <message>" for either. Anything that escapes is "E" and exit 2: CQT
 * leaves threads that keep a JVM alive after main returns, and a JVM that
 * neither answers nor exits would hang the run.
 *
 * testkit.follow_order=true, for a run with slots, makes each case start with
 * server messages where CTP's single run would have them (serverMessageInOrder).
 *
 * The source is embedded in the runner and compiled against the CTP it runs
 * with (internal/runner/sqlsuite/executor.go).
 */
public class TestkitExecutor {
    public static void main(String[] args) throws Exception {
        PrintStream proto = new PrintStream(new FileOutputStream("/dev/fd/3"), false, "UTF-8");
        try {
            run(args, proto);
        } catch (Throwable t) {
            proto.println("E " + oneLine(t));
            proto.flush();
            System.exit(2);
        }
        // CQT leaves non-daemon threads behind (its connections), so returning
        // from main would leave the JVM running.
        System.exit(0);
    }

    private static void run(String[] args, PrintStream proto) throws Exception {
        PrintStream out = System.out;
        ByteArrayOutputStream startup = new ByteArrayOutputStream();
        System.setOut(new PrintStream(startup, true, "UTF-8"));

        String type = args[0], typeAlias = args[1], version = args[2], charsetXml = args[3];
        String[] files = new String[args.length - 4];
        System.arraycopy(args, 4, files, 0, files.length);
        boolean bits32 = "32".equalsIgnoreCase(version);

        // ConsoleAgent.runTest, as runCQT calls it: printResult false.
        String testId = TestUtil.getTestId("schedule", type + (bits32 ? "_32bit" : "_64bit"));
        Test test = new Test(testId);
        test.setRunMode(Test.MODE_RESULT);
        test.setCodeset(TestUtil.DEFAULT_CODESET);
        test.setTestType(type);
        test.setResult_dir(System.getProperty("testkit.result_dir", TestUtil.getResultDir(testId)));
        test.setTestTypeAlias(typeAlias);
        test.setTestBit(bits32 ? "32bit" : "64bit");
        test.setCharset_file(charsetXml);
        PropertiesUtil.initConfig(TestUtil.getCharsetFile(charsetXml), test);
        test.setDebug(Boolean.parseBoolean(PropertiesUtil.getValueWithDefault("isdebug", "false").trim()));
        try {
            if (Boolean.parseBoolean(PropertiesUtil.getValueWithDefault("qaview", "false").trim())) {
                test.setQaview(true);
            }
        } catch (Exception e) {
            System.err.println("There is an exception when getting variable 'qaview' from local.properties:  \n"
                    + e.getMessage());
        }
        if (bits32) {
            test.setVersion("32bits");
        } else if ("windows".equalsIgnoreCase(SystemUtil.getOS())) {
            test.setVersion("64bits");
        } else {
            test.setVersion("Main");
        }
        test.setCases(files.clone());

        // ConsoleBO.runTest up to its loop: init, the DAO, discovery, the db check.
        ConsoleBO bo = new ConsoleBO(false, true);
        ConfigureUtil configureUtil = new ConfigureUtil();
        set(bo, "test", test);
        set(bo, "configureUtil", configureUtil);
        call(bo, "init");
        set(bo, "dao", new ConsoleDAO(test, configureUtil));
        call(bo, "buildTest", test);
        if (!(Boolean) call(bo, "checkDb", test)) {
            System.err.write(startup.toByteArray());
            System.err.flush();
            throw new IllegalStateException("checkDb failed: CQT cannot reach the database");
        }
        // Every name looked up by reflection, before READY: a CQT that differs is
        // a run that does not start, not a case that fails halfway through.
        Method execute = ConsoleBO.class.getDeclaredMethod("executeSqlFile", Test.class, CaseResult.class);
        execute.setAccessible(true);
        Method failure = JunitXmlWriter.class.getDeclaredMethod("buildFailureCdata", CaseResult.class, String.class);
        failure.setAccessible(true);
        String logId = (String) get(bo, "logId");
        boolean followOrder = Boolean.getBoolean("testkit.follow_order");
        Map<String, String> serverMessageAt =
                followOrder ? serverMessageInOrder(bo, test) : new HashMap<String, String>();
        System.out.flush();
        byte[] said = startup.toByteArray();
        System.setOut(out);
        proto.println("READY " + test.getCaseFileList().size() + " " + said.length);
        proto.write(said);
        proto.flush();

        BufferedReader in = new BufferedReader(new InputStreamReader(System.in, "UTF-8"));
        for (String line; (line = in.readLine()) != null; ) {
            String caseFile = line.length() > 2 ? line.substring(2) : "";
            CaseResult caseResult = test.getCaseResultFromMap(caseFile);
            if (caseResult == null) {
                proto.println("E not a case of this run: " + caseFile);
                proto.flush();
                continue;
            }
            try {
                if (line.startsWith("X ")) {
                    String want = serverMessageAt.get(caseFile);
                    if (want != null && !want.equalsIgnoreCase(test.getServerMessage())) {
                        try {
                            replayServerMessage(bo, test, execute, want, caseFile);
                        } catch (Throwable t) {
                            // The case still runs; what it prints about its
                            // errors may then differ from CTP's, and this says why.
                            System.err.println("[testkit] cannot put server messages " + want
                                    + " before " + caseFile + ": " + oneLine(t));
                        }
                    }
                    execute.invoke(bo, test, caseResult);
                    if (caseResult.getResult() == null) {
                        // SQLParser could not read the file. CQT's own loop dies on
                        // this a line later, comparing a null.
                        proto.println("E CQT could not read the case");
                        proto.flush();
                        continue;
                    }
                    reply(proto, caseResult.getResult(), caseResult.getTotalTime());
                    // What CQT's loop does once the result is saved: the text is
                    // not needed again, and a run holds 17,000 of them.
                    caseResult.setResult("");
                } else if (line.startsWith("F ")) {
                    reply(proto, (String) failure.invoke(null, caseResult, logId), 0);
                } else {
                    proto.println("E no such request: " + line);
                }
            } catch (Throwable t) {
                // executeSqlFile catches what a statement throws and renders it,
                // so this is CQT itself failing -- which in CQT's loop ends the
                // run. Here it ends the case, and the runner says so.
                proto.println("E " + oneLine(t instanceof InvocationTargetException ? t.getCause() : t));
            }
            proto.flush();
        }
    }

    /**
     * Whether server messages are on when each case starts, in CQT's order.
     *
     * It is the one thing CQT carries from case to case of its own accord:
     * resetConnection puts autocommit, holdcas and the reset script back before
     * every case, and leaves test.serverMessage as the last "--+ server-message"
     * hint left it. So in CTP's single run a case prints its errors' messages or
     * not according to every hint before it -- and a slot runs a different "every
     * case before it". Measured: of 497 verdicts eight slots moved, 495 were this.
     *
     * The value is worked out the way executeSqlFile works it out: CQT's parser,
     * isPropOn and isPropOff, in their else-if order (autocommit and holdcas
     * first), over every case that runs. Only a file that names the hint can move
     * it, so only those are parsed.
     */
    private static Map<String, String> serverMessageInOrder(ConsoleBO bo, Test test) throws Exception {
        Method on = ConsoleBO.class.getDeclaredMethod("isPropOn", String.class, String.class);
        Method off = ConsoleBO.class.getDeclaredMethod("isPropOff", String.class, String.class);
        on.setAccessible(true);
        off.setAccessible(true);
        Map<String, String> at = new HashMap<String, String>();
        String state = test.getServerMessage();
        for (Object o : test.getCaseFileList()) {
            String caseFile = (String) o;
            at.put(caseFile, state);
            CaseResult cr = test.getCaseResultFromMap(caseFile);
            if (cr == null || !cr.isShouldRun()) {
                continue;
            }
            List<Sql> sqls;
            try {
                byte[] raw = Files.readAllBytes(Paths.get(caseFile));
                if (!new String(raw, "UTF-8").toLowerCase().contains(TestUtil.SERVER_MESSAGE)) {
                    continue;
                }
                sqls = SQLParser.parseSqlFile(caseFile, test.getCodeset(), test.isNeedDebugHint());
            } catch (Exception e) {
                // A file CQT cannot read runs no statement, and moves nothing.
                continue;
            }
            if (sqls == null) {
                continue;
            }
            for (Sql sql : sqls) {
                String script = sql.getScript();
                if ((Boolean) on.invoke(bo, TestUtil.AUTOCOMMIT, script)
                        || (Boolean) off.invoke(bo, TestUtil.AUTOCOMMIT, script)
                        || (Boolean) on.invoke(bo, TestUtil.HOLDCAS, script)
                        || (Boolean) off.invoke(bo, TestUtil.HOLDCAS, script)) {
                    continue;
                }
                if ((Boolean) on.invoke(bo, TestUtil.SERVER_MESSAGE, script)) {
                    state = "on";
                } else if ((Boolean) off.invoke(bo, TestUtil.SERVER_MESSAGE, script)) {
                    state = "off";
                }
            }
        }
        return at;
    }

    /**
     * Puts server messages where CTP's order would have them, by running the hint
     * through CQT's own code: a one-line case, "--+ server-message on|off", on the
     * database the next case uses. That is what sets the flag and enables or
     * disables DBMS_OUTPUT on the connection, exactly as a case carrying the hint
     * does; the rest of what executeSqlFile does around it -- the reset, the
     * server check, the commits -- the next case does again anyway.
     */
    private static void replayServerMessage(ConsoleBO bo, Test test, Method execute, String want, String next)
            throws Exception {
        File hint = File.createTempFile("testkit-server-message-", ".sql");
        try {
            Files.write(hint.toPath(), ("--+ server-message " + want + "\n").getBytes("UTF-8"));
            @SuppressWarnings("unchecked")
            Map<Object, Object> caseDb = (Map<Object, Object>) get(test, "caseDbMap");
            caseDb.put(hint.getPath(), test.getDbId(next));
            CaseResult cr = new CaseResult();
            cr.setCaseFile(hint.getPath());
            cr.setCaseDir(hint.getParent());
            cr.setCaseName(hint.getName());
            cr.setType(CaseResult.TYPE_SQL);
            try {
                execute.invoke(bo, test, cr);
            } finally {
                caseDb.remove(hint.getPath());
            }
        } finally {
            hint.delete();
        }
    }

    private static void reply(PrintStream proto, String text, long ms) throws Exception {
        byte[] bytes = (text == null ? "" : text).getBytes("UTF-8");
        proto.println("R " + bytes.length + " " + ms);
        proto.write(bytes);
    }

    private static String oneLine(Throwable t) {
        StringWriter w = new StringWriter();
        t.printStackTrace(new PrintWriter(w));
        System.err.print(w);
        return (t.getClass().getName() + ": " + t.getMessage()).replace('\n', ' ').replace('\r', ' ');
    }

    private static void set(Object o, String field, Object value) throws Exception {
        Field f = o.getClass().getDeclaredField(field);
        f.setAccessible(true);
        f.set(o, value);
    }

    private static Object get(Object o, String field) throws Exception {
        for (Class<?> c = o.getClass(); c != null; c = c.getSuperclass()) {
            try {
                Field f = c.getDeclaredField(field);
                f.setAccessible(true);
                return f.get(o);
            } catch (NoSuchFieldException e) {
                // declared higher up
            }
        }
        throw new NoSuchFieldException(field);
    }

    private static Object call(Object o, String name, Object... args) throws Exception {
        for (Class<?> c = o.getClass(); c != null; c = c.getSuperclass()) {
            for (Method m : c.getDeclaredMethods()) {
                if (m.getName().equals(name) && m.getParameterTypes().length == args.length) {
                    m.setAccessible(true);
                    return m.invoke(o, args);
                }
            }
        }
        throw new NoSuchMethodException(name);
    }
}
