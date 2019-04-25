package main

import (
	"crypto/tls"
	"fmt"
	"gopkg.in/ldap.v2"
	"net"
	"reflect"
)

const _PAGING_SIZE = 500

// LdapInfo contains connection info
type LdapInfo struct {
	LdapServer  string
	LdapIP      string
	LdapPort    uint16
	LdapTLSPort uint16
	User        string
	Usergpp     string
	Pass        string
	Domain      string
	Unsafe      bool
	StartTLS    bool
}

func dial(li *LdapInfo) (*ldap.Conn, error) {
	if li.Unsafe {
		fmt.Printf("[i] Begin PLAINTEXT LDAP connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapPort))
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] PLAINTEXT LDAP connection to '%s' (%s) successful...\n", li.LdapServer, li.LdapIP)
		return conn, nil
	} else if li.StartTLS {
		fmt.Printf("[i] Begin PLAINTEXT LDAP connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapPort))
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] PLAINTEXT LDAP connection to '%s' (%s) successful...\n[i] Upgrade to StartTLS connection...\n", li.LdapServer, li.LdapIP)
		err = conn.StartTLS(&tls.Config{ServerName: li.LdapServer})
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] Upgrade to StartTLS connection successful...\n")
		return conn, nil
	} else {
		fmt.Printf("[i] Begin LDAP TLS connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		config := &tls.Config{ServerName: li.LdapServer}
		conn, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapTLSPort), config)
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] LDAP TLS connection to '%s' (%s) successful...\n", li.LdapServer, li.LdapIP)
		return conn, nil
	}
}

// Connect authenticated bind to ldap connection
func LdapConnect(li *LdapInfo) (*ldap.Conn, error) {
	conn, err := dial(li)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[i] Begin BIND...\n")
	err = conn.Bind(li.User, li.Pass)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[i] BIND with '%s' successful...\n[i] Begin dump domain info...\n", li.User)
	return conn, nil
}

func NewConnectionInfo(ldapServer string, domain string, user string, pass string, unsafe bool, tls bool) (*LdapInfo, error) {
	addr, err := net.LookupIP(ldapServer)
	if err != nil {
		return nil, err
	}
	ldapIP := addr[0].String()

	li := &LdapInfo{
		LdapServer:  ldapServer,
		LdapIP:      ldapIP,
		LdapPort:    uint16(389),
		LdapTLSPort: uint16(636),
		User:        user + "@" + domain,
		Usergpp:     user,
		Pass:        pass,
		Domain:      domain,
		Unsafe:      unsafe,
		StartTLS:    tls}
	return li, nil
}

type LdapUserInfo struct {
	objectSid         string
	sAMAccountName    string
	sAMAccountType    string
	userPrincipalName string
	displayName       string
	givenName         string
	description       string
	adminCount        string
	homeDirectory     string
}

func GetStructFields(t interface{}) []string {
	s := reflect.ValueOf(t).Elem()
	typeOfT := s.Type()
	ret := []string{}
	for i := 0; i < s.NumField(); i++ {
		// f := s.Field(i)
		ret = append(ret, typeOfT.Field(i).Name)
	}
	return ret
}

func SetStructStringField(t interface{}, field string, value string) error {
	// Elem returns the value that the pointer u points to.
	v := reflect.ValueOf(t).Elem()
	f := v.FieldByName(field)
	if !f.IsValid() || !f.CanSet() {
		return fmt.Errorf("Could not set field %v", field)
	}
	if f.Kind() != reflect.String || f.String() != "" {
		return fmt.Errorf("Field %v is not string", field)
	}
	f.SetString(value)
	return nil
}

// Helper function for LDAP search
func LdapSearch(searchDN string, filter string, attributes []string, conn *ldap.Conn) (*ldap.SearchResult, error) {
	searchRequest := ldap.NewSearchRequest(
		searchDN,
		ldap.ScopeWholeSubtree, ldap.DerefAlways, 0, 0, false,
		filter,
		attributes,
		nil)

	sr, err := conn.SearchWithPaging(searchRequest, _PAGING_SIZE)
	if err != nil {
		return nil, err
	}
	return sr, nil
}

func LdapSearchUsers(conn *ldap.Conn, searchDN string, filter string) ([]LdapUserInfo, error) {
	if filter == "" {
		filter = "(&(objectCategory=person)(objectClass=user)(SamAccountName=*))"
	}
	attributes := GetStructFields(&LdapUserInfo{})
	sr, err := LdapSearch(searchDN, filter, attributes, conn)
	if err != nil {
		return nil, err
	}

	ret := []LdapUserInfo{}
	for _, entry := range sr.Entries {
		// mem := strings.Join(entry.GetAttributeValues("memberOf"), " ")
		info := LdapUserInfo{}
		for _, fld := range GetStructFields(&info) {
			val := entry.GetAttributeValue(fld)
			err := SetStructStringField(&info, fld, val)
			if err != nil {
				return nil, err
			}
		}
		ret = append(ret, info)
	}
	return ret, nil
}