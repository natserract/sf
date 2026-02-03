public with sharing class StringListTest {
    public StringListTest(Integer integerParam) {
        List<String> resultList = new List<String>();
        for (Integer i = 0; i < integerParam; i++) {
            resultList.add('Test ' + i);
        }
    }
}