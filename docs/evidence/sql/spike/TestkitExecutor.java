import com.navercorp.cubridqa.cqt.console.bean.CaseResult;
import com.navercorp.cubridqa.cqt.console.bean.Test;
import com.navercorp.cubridqa.cqt.console.bo.ConsoleBO;
import com.navercorp.cubridqa.cqt.console.dao.ConsoleDAO;
import com.navercorp.cubridqa.cqt.console.util.ConfigureUtil;
import com.navercorp.cubridqa.cqt.console.util.PropertiesUtil;
import com.navercorp.cubridqa.cqt.console.util.TestUtil;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.lang.reflect.Field;
import java.lang.reflect.Method;

/**
 * CQT without its loop: set a run up exactly as ConsoleAgent.runTest does, then
 * execute whichever case the caller names, one at a time, and hand back the text
 * CQT would have written to that case's .result.
 *
 *   java TestkitExecutor <testType> <typeAlias> <charset_xml> <scenario>?db=<db>_qa[&filter=..]
 *
 * Protocol, on standard output: one request per line on standard input (a case
 * path); for each, "R <n>\n" and n bytes of UTF-8, or "E <message>\n". CQT's own
 * messages go to standard error, which is where System.out points while it runs.
 */
public class TestkitExecutor {
    public static void main(String[] args) throws Exception {
        PrintStream proto = System.out;
        System.setOut(System.err);

        String testType = args[0], typeAlias = args[1], charsetFile = args[2], scenario = args[3];

        // ConsoleAgent.runTest, COME_FROM_CQT_64, printResult=true.
        String testId = TestUtil.getTestId("schedule", testType + "_64bit");
        Test test = new Test(testId);
        test.setRunMode(Test.MODE_RESULT);
        test.setCodeset(TestUtil.DEFAULT_CODESET);
        test.setTestType(testType);
        test.setResult_dir(TestUtil.getResultDir(testId));
        test.setTestTypeAlias(typeAlias);
        test.setTestBit("64bit");
        test.setCharset_file(charsetFile);
        PropertiesUtil.initConfig(TestUtil.getCharsetFile(charsetFile), test);
        test.setDebug(Boolean.parseBoolean(PropertiesUtil.getValueWithDefault("isdebug", "false").trim()));
        test.setVersion("Main");
        test.setCases(new String[] {scenario});

        // ConsoleBO.runTest up to the loop: init, the DAO, discovery, the db check.
        ConsoleBO bo = new ConsoleBO(false, true);
        ConfigureUtil configureUtil = new ConfigureUtil();
        set(bo, "test", test);
        set(bo, "configureUtil", configureUtil);
        call(bo, "init");
        set(bo, "dao", new ConsoleDAO(test, configureUtil));
        call(bo, "buildTest", test);
        if (!(Boolean) call(bo, "checkDb", test)) {
            proto.println("E checkDb failed");
            proto.flush();
            System.exit(2);
        }
        proto.println("READY " + test.getCaseFileList().size());
        proto.flush();

        Method execute = ConsoleBO.class.getDeclaredMethod("executeSqlFile", Test.class, CaseResult.class);
        execute.setAccessible(true);
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in, "UTF-8"));
        for (String caseFile; (caseFile = in.readLine()) != null; ) {
            CaseResult caseResult = test.getCaseResultFromMap(caseFile);
            if (caseResult == null) {
                proto.println("E not a case of this run: " + caseFile);
            } else {
                execute.invoke(bo, test, caseResult);
                String result = caseResult.getResult();
                byte[] bytes = (result == null ? "" : result).getBytes("UTF-8");
                proto.println("R " + bytes.length);
                proto.write(bytes);
                caseResult.setResult("");
            }
            proto.flush();
        }
        // CQT leaves non-daemon threads behind (its stdout job, connection
        // threads), so returning from main would leave the JVM running.
        System.exit(0);
    }

    private static void set(Object o, String field, Object value) throws Exception {
        Field f = o.getClass().getDeclaredField(field);
        f.setAccessible(true);
        f.set(o, value);
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
